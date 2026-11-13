/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/moby/moby/client/pkg/versions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v5/internal/concurrency"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) ensureModels(ctx context.Context, project *types.Project, quietPull bool) error {
	if len(project.Models) == 0 {
		return nil
	}

	mdlAPI, err := s.newModelAPI(project)
	if err != nil {
		return err
	}
	defer mdlAPI.Close()
	if err := mdlAPI.Check(ctx); err != nil {
		return err
	}
	// best effort: when the models cannot be listed, they are all pulled
	availableModels, _ := mdlAPI.ListModels(ctx)

	eg, ctx := concurrency.NewErrgroup(ctx, s.maxConcurrency)
	eg.Go(func() error {
		return mdlAPI.SetModelVariables(ctx, project)
	})

	for name, config := range project.Models {
		if config.Name == "" {
			config.Name = name
		}
		eg.Go(func() error {
			available, err := mdlAPI.HasModel(ctx, availableModels, config.Model)
			if err != nil {
				return err
			}
			if !available {
				err = mdlAPI.PullModel(ctx, config, quietPull, s.events)
				if err != nil {
					return err
				}
			}
			return mdlAPI.ConfigureModel(ctx, config, s.events)
		})
	}
	return eg.Wait()
}

// modelAPI is a backend able to serve the top-level `models` element.
// Docker Model Runner is the default one, llmman can be selected with
// COMPOSE_MODEL_PROVIDER or is used as a fallback when the Docker Model
// plugin is not installed (see model_llmman.go).
type modelAPI interface {
	// Check verifies the backend is usable before any model is listed or pulled.
	Check(ctx context.Context) error
	// ListModels returns the models already available locally. Its error is not fatal.
	ListModels(ctx context.Context) ([]string, error)
	// HasModel tells whether model is available, given the result of ListModels.
	HasModel(ctx context.Context, available []string, model string) (bool, error)
	// PullModel fetches a model which is not available yet.
	PullModel(ctx context.Context, model types.ModelConfig, quietPull bool, events api.EventProcessor) error
	// ConfigureModel applies the model settings (context size, runtime flags).
	ConfigureModel(ctx context.Context, config types.ModelConfig, events api.EventProcessor) error
	// SetModelVariables injects the model and endpoint variables into services.
	SetModelVariables(ctx context.Context, project *types.Project) error
	// Close releases resources held by the backend.
	Close()
}

const (
	// modelProviderEnv selects the models backend, read from the project environment.
	// When unset or empty, Docker Model Runner is used, or llmman if the Docker Model
	// plugin is not installed and llmman is found in PATH. Setting it explicitly
	// disables that fallback.
	modelProviderEnv = "COMPOSE_MODEL_PROVIDER"
	// modelProviderDMR is the default models backend, Docker Model Runner.
	modelProviderDMR = "docker-model-runner"
	// modelProviderLLMMan selects llmman, see model_llmman.go.
	modelProviderLLMMan = "llmman"
)

// errDMRPluginNotFound is returned when the `docker model` CLI plugin is not installed.
var errDMRPluginNotFound = errors.New("'models' support requires Docker Model plugin")

// newModelAPI returns the backend selected by COMPOSE_MODEL_PROVIDER. When the
// variable is unset or empty, Docker Model Runner is used. Only if its plugin
// is not installed (a broken plugin is an error) and an llmman binary is found
// in PATH is llmman used instead. An explicit value never falls back.
func (s *composeService) newModelAPI(project *types.Project) (modelAPI, error) {
	switch provider := strings.TrimSpace(project.Environment[modelProviderEnv]); provider {
	case "", modelProviderDMR:
		m, err := s.newDMRModelAPI(project)
		if err != nil {
			if provider == "" && errors.Is(err, errDMRPluginNotFound) {
				if path, lookErr := exec.LookPath(llmmanBinary); lookErr == nil {
					logrus.Debugf("Docker Model plugin not found, using llmman (%s) for models", path)
					return s.newLLMManModelAPI(project)
				}
			}
			return nil, err
		}
		return m, nil
	case modelProviderLLMMan:
		m, err := s.newLLMManModelAPI(project)
		if err != nil {
			return nil, err
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported %s %q: supported values are %q (default) and %q",
			modelProviderEnv, provider, modelProviderDMR, modelProviderLLMMan)
	}
}

// dmrModelAPI is the Docker Model Runner backend, driven through the `docker model` CLI plugin.
type dmrModelAPI struct {
	path    string
	env     []string
	prepare func(ctx context.Context, cmd *exec.Cmd) error
	cleanup func()
	version string
	dryRun  bool
}

func (s *composeService) newDMRModelAPI(project *types.Project) (*dmrModelAPI, error) {
	dockerModel, err := manager.GetPlugin("model", s.dockerCli, &cobra.Command{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, errDMRPluginNotFound
		}
		return nil, err
	}
	if dockerModel.Err != nil {
		return nil, fmt.Errorf("failed to load Docker Model plugin: %w", dockerModel.Err)
	}
	endpoint, cleanup, err := s.propagateDockerEndpoint()
	if err != nil {
		return nil, err
	}
	return &dmrModelAPI{
		path:    dockerModel.Path,
		version: dockerModel.Version,
		prepare: func(ctx context.Context, cmd *exec.Cmd) error {
			return s.prepareShellOut(ctx, project.Environment, cmd)
		},
		cleanup: cleanup,
		env:     append(project.Environment.Values(), endpoint...),
		dryRun:  s.dryRun,
	}, nil
}

func (m *dmrModelAPI) Close() {
	m.cleanup()
}

// Check is a no-op: the plugin is validated when loaded.
func (m *dmrModelAPI) Check(context.Context) error {
	return nil
}

func (m *dmrModelAPI) HasModel(_ context.Context, available []string, model string) (bool, error) {
	return slices.Contains(available, model), nil
}

func (m *dmrModelAPI) PullModel(ctx context.Context, model types.ModelConfig, quietPull bool, events api.EventProcessor) error {
	if !startPull(model.Name, m.dryRun, events) {
		return nil
	}

	cmd := exec.CommandContext(ctx, m.path, "pull", model.Model)
	err := m.prepare(ctx, cmd)
	if err != nil {
		return err
	}
	err = streamPull(cmd, model.Name, quietPull, events)
	if err != nil {
		events.On(progress.Error(model.Name, err.Error()))
	}
	events.On(api.Resource{
		ID:     model.Name,
		Status: api.Working,
		Text:   api.StatusPulled,
	})
	return err
}

// startPull reports a pull as started and tells whether to go on. The backends
// are external processes the dry-run API client cannot intercept: on a dry run
// the pull is reported as done and nothing is run.
func startPull(name string, dryRun bool, events api.EventProcessor) bool {
	events.On(api.Resource{ID: name, Status: api.Working, Text: api.StatusPulling})
	if dryRun {
		events.On(api.Resource{ID: name, Status: api.Working, Text: api.StatusPulled})
	}
	return !dryRun
}

// streamPull runs a pull command and reports a pulling event for each line it
// prints on stdout, unless quiet.
func streamPull(cmd *exec.Cmd, name string, quiet bool, events api.EventProcessor) error {
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stream.Close()
		return err
	}

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		if scanner.Text() == "" || quiet {
			continue
		}
		events.On(api.Resource{ID: name, Status: api.Working, Text: api.StatusPulling})
	}
	if scanner.Err() != nil {
		// A line too long for the scanner stops it reading: drain the rest so
		// the child does not block on a full pipe.
		_, _ = io.Copy(io.Discard, stream)
	}
	return cmd.Wait()
}

func (m *dmrModelAPI) ConfigureModel(ctx context.Context, config types.ModelConfig, events api.EventProcessor) error {
	events.On(api.Resource{
		ID:     config.Name,
		Status: api.Working,
		Text:   api.StatusConfiguring,
	})
	if m.dryRun {
		events.On(api.Resource{ID: config.Name, Status: api.Done, Text: api.StatusConfigured})
		return nil
	}
	// configure [--context-size=<n>] MODEL [-- <runtime-flags...>]
	args := []string{"configure"}
	if config.ContextSize > 0 {
		args = append(args, "--context-size", strconv.Itoa(config.ContextSize))
	}
	args = append(args, config.Model)
	// Only append RuntimeFlags if docker model CLI version is >= v1.0.6
	if len(config.RuntimeFlags) != 0 && m.supportsRuntimeFlags() {
		args = append(args, "--")
		args = append(args, config.RuntimeFlags...)
	}
	cmd := exec.CommandContext(ctx, m.path, args...)
	err := m.prepare(ctx, cmd)
	if err != nil {
		return err
	}
	err = cmd.Run()
	if err != nil {
		events.On(progress.Error(config.Name, err.Error()))
		return err
	}
	events.On(api.Resource{
		ID:     config.Name,
		Status: api.Done,
		Text:   api.StatusConfigured,
	})
	return nil
}

func (m *dmrModelAPI) SetModelVariables(ctx context.Context, project *types.Project) error {
	cmd := exec.CommandContext(ctx, m.path, "status", "--json")
	err := m.prepare(ctx, cmd)
	if err != nil {
		return err
	}

	statusOut, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking docker-model status: %w", err)
	}
	type Status struct {
		Endpoint string `json:"endpoint"`
	}

	var status Status
	err = json.Unmarshal(statusOut, &status)
	if err != nil {
		return err
	}

	setModelVariables(project, status.Endpoint)
	return nil
}

// setModelVariables injects, for each model bound to a service, the model name and
// endpoint variables (<NAME>_MODEL and <NAME>_URL, or the model_var and endpoint_var overrides).
func setModelVariables(project *types.Project, endpoint string) {
	for _, service := range project.Services {
		for ref, modelConfig := range service.Models {
			model := project.Models[ref]
			varPrefix := strings.ReplaceAll(strings.ToUpper(ref), "-", "_")
			var variable string
			if modelConfig != nil && modelConfig.ModelVariable != "" {
				variable = modelConfig.ModelVariable
			} else {
				variable = varPrefix + "_MODEL"
			}
			service.Environment[variable] = &model.Model

			if modelConfig != nil && modelConfig.EndpointVariable != "" {
				variable = modelConfig.EndpointVariable
			} else {
				variable = varPrefix + "_URL"
			}
			service.Environment[variable] = &endpoint
		}
	}
}

type Model struct {
	Id      string   `json:"id"`
	Tags    []string `json:"tags"`
	Created int      `json:"created"`
	Config  struct {
		Format       string `json:"format"`
		Quantization string `json:"quantization"`
		Parameters   string `json:"parameters"`
		Architecture string `json:"architecture"`
		Size         string `json:"size"`
	} `json:"config"`
}

func (m *dmrModelAPI) ListModels(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, m.path, "ls", "--json")
	err := m.prepare(ctx, cmd)
	if err != nil {
		return nil, err
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error checking available models: %w", err)
	}

	type AvailableModel struct {
		Id      string   `json:"id"`
		Tags    []string `json:"tags"`
		Created int      `json:"created"`
	}

	models := []AvailableModel{}
	err = json.Unmarshal(output, &models)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling available models: %w", err)
	}
	var availableModels []string
	for _, model := range models {
		availableModels = append(availableModels, model.Tags...)
	}
	return availableModels, nil
}

// supportsRuntimeFlags checks if the docker model version supports runtime flags
// Runtime flags are supported in version >= v1.0.6
func (m *dmrModelAPI) supportsRuntimeFlags() bool {
	// If version is not cached, don't append runtime flags to be safe
	if m.version == "" {
		return false
	}

	// Strip 'v' prefix if present (e.g., "v1.0.6" -> "1.0.6")
	versionStr := strings.TrimPrefix(m.version, "v")
	return !versions.LessThan(versionStr, "1.0.6")
}
