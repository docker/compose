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
	env     []string
	prepare func(ctx context.Context, cmd *exec.Cmd) error
	cleanup func()
}

func (s *composeService) newModelAPI(project *types.Project) (*modelAPI, error) {
	dockerModel, err := manager.GetPlugin("model", s.dockerCli, &cobra.Command{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("'models' support requires Docker Model plugin")
		}
		return nil, err
	}
	endpoint, cleanup, err := s.propagateDockerEndpoint()
	if err != nil {
		return nil, err
	}
	return &modelAPI{
		path: dockerModel.Path,
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
	if len(config.RuntimeFlags) != 0 && m.supportsRuntimeFlags(ctx) {
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

// getModelVersion retrieves the docker model CLI version
func (m *modelAPI) getModelVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, m.path, "version")
	err := m.prepare(ctx, cmd)
	if err != nil {
		return "", err
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error getting docker model version: %w", err)
	}

	// Parse output like: "Docker Model Runner version v1.0.4"
	// We need to extract the version string (e.g., "v1.0.4")
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.Contains(line, "version") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "version" && i+1 < len(parts) {
					return parts[i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not parse docker model version from output: %s", string(output))
}

// supportsRuntimeFlags checks if the docker model version supports runtime flags
// Runtime flags are supported in version >= v1.0.6
func (m *modelAPI) supportsRuntimeFlags(ctx context.Context) bool {
	versionStr, err := m.getModelVersion(ctx)
	if err != nil {
		// If we can't determine the version, don't append runtime flags to be safe
		return false
	}

	// Parse version strings
	currentVersion, err := parseVersion(versionStr)
	if err != nil {
		return false
	}

	minVersion, err := parseVersion("1.0.6")
	if err != nil {
		return false
	}

	return !currentVersion.LessThan(minVersion)
}

// parseVersion parses a semantic version string
// Strips build metadata and prerelease suffixes (e.g., "1.0.6-dirty" or "1.0.6+build")
func parseVersion(versionStr string) (*semVersion, error) {
	// Remove 'v' prefix if present
	versionStr = strings.TrimPrefix(versionStr, "v")

	// Strip build metadata or prerelease suffix after "-" or "+"
	// Examples: "1.0.6-dirty" -> "1.0.6", "1.0.6+build" -> "1.0.6"
	if idx := strings.IndexAny(versionStr, "-+"); idx != -1 {
		versionStr = versionStr[:idx]
	}

	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version format: %s", versionStr)
	}

	var v semVersion
	var err error

	v.major, err = strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", parts[0])
	}

	v.minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	if len(parts) > 2 {
		v.patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %s", parts[2])
		}
	}

	return &v, nil
}

// semVersion represents a semantic version
type semVersion struct {
	major int
	minor int
	patch int
}

// LessThan compares two semantic versions
func (v *semVersion) LessThan(other *semVersion) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	return v.patch < other.patch
}
