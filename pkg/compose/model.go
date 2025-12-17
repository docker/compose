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
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/docker/api/types/versions"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

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
	availableModels, err := mdlAPI.ListModels(ctx)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return mdlAPI.SetModelVariables(ctx, project)
	})

	for name, config := range project.Models {
		if config.Name == "" {
			config.Name = name
		}
		eg.Go(func() error {
			if !slices.Contains(availableModels, config.Model) {
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

type modelAPI struct {
	path    string
	version string // cached plugin version
	env     []string
	prepare func(ctx context.Context, cmd *exec.Cmd) error
	cleanup func()
	version string
}

func (s *composeService) newModelAPI(project *types.Project) (*modelAPI, error) {
	dockerModel, err := manager.GetPlugin("model", s.dockerCli, &cobra.Command{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("'models' support requires Docker Model plugin")
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
	return &modelAPI{
		path:    dockerModel.Path,
		version: dockerModel.Version,
		prepare: func(ctx context.Context, cmd *exec.Cmd) error {
			return s.prepareShellOut(ctx, project.Environment, cmd)
		},
		cleanup: cleanup,
		env:     append(project.Environment.Values(), endpoint...),
	}, nil
}

func (m *modelAPI) Close() {
	m.cleanup()
}

func (m *modelAPI) PullModel(ctx context.Context, model types.ModelConfig, quietPull bool, events api.EventProcessor) error {
	events.On(api.Resource{
		ID:     model.Name,
		Status: api.Working,
		Text:   api.StatusPulling,
	})

	cmd := exec.CommandContext(ctx, m.path, "pull", model.Model)
	err := m.prepare(ctx, cmd)
	if err != nil {
		return err
	}
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		msg := scanner.Text()
		if msg == "" {
			continue
		}

		if !quietPull {
			events.On(api.Resource{
				ID:     model.Name,
				Status: api.Working,
				Text:   api.StatusPulling,
			})
		}
	}

	err = cmd.Wait()
	if err != nil {
		events.On(errorEvent(model.Name, err.Error()))
	}
	events.On(api.Resource{
		ID:     model.Name,
		Status: api.Working,
		Text:   api.StatusPulled,
	})
	return err
}

func (m *modelAPI) ConfigureModel(ctx context.Context, config types.ModelConfig, events api.EventProcessor) error {
	events.On(api.Resource{
		ID:     config.Name,
		Status: api.Working,
		Text:   api.StatusConfiguring,
	})
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
		events.On(errorEvent(config.Name, err.Error()))
		return err
	}
	events.On(api.Resource{
		ID:     config.Name,
		Status: api.Done,
		Text:   api.StatusConfigured,
	})
	return nil
}

func (m *modelAPI) SetModelVariables(ctx context.Context, project *types.Project) error {
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
			service.Environment[variable] = &status.Endpoint
		}
	}
	return nil
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

func (m *modelAPI) ListModels(ctx context.Context) ([]string, error) {
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
func (m *modelAPI) supportsRuntimeFlags() bool {
	// If version is not cached, don't append runtime flags to be safe
	if m.version == "" {
		return false
	}

	// Strip 'v' prefix if present (e.g., "v1.0.6" -> "1.0.6")
	versionStr := strings.TrimPrefix(m.version, "v")

	// Strip build metadata or prerelease suffix after "-" or "+"
	// This is necessary because versions.LessThan treats "1.0.6-dirty" < "1.0.6" per semver rules
	// but we want to compare the base version numbers only
	if idx := strings.IndexAny(versionStr, "-+"); idx != -1 {
		versionStr = versionStr[:idx]
	}

	return !versions.LessThan(versionStr, "1.0.6")
}
