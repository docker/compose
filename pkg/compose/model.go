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
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) ensureModels(ctx context.Context, project *types.Project, quietPull bool) error {
	if len(project.Models) == 0 {
		return nil
	}

	dockerModel, err := manager.GetPlugin("model", s.dockerCli, &cobra.Command{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("'models' support requires Docker Model plugin")
		}
		return err
	}

	cmd := exec.CommandContext(ctx, dockerModel.Path, "ls", "--json")
	s.setupChildProcess(ctx, cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking available models: %w", err)
	}

	type AvailableModel struct {
		Id      string   `json:"id"`
		Tags    []string `json:"tags"`
		Created int      `json:"created"`
	}

	models := []AvailableModel{}
	err = json.Unmarshal(output, &models)
	if err != nil {
		return fmt.Errorf("error unmarshalling available models: %w", err)
	}
	var availableModels []string
	for _, model := range models {
		availableModels = append(availableModels, model.Tags...)
	}

	eg, gctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return s.setModelVariables(gctx, dockerModel, project)
	})

	for name, config := range project.Models {
		if config.Name == "" {
			config.Name = name
		}
		eg.Go(func() error {
			w := progress.ContextWriter(gctx)
			if !slices.Contains(availableModels, config.Model) {
				err = s.pullModel(gctx, dockerModel, config, quietPull, w)
				if err != nil {
					return err
				}
			}
			err = s.configureModel(gctx, dockerModel, config, w)
			if err != nil {
				return err
			}
			w.Event(progress.CreatedEvent(config.Name))
			return nil
		})
	}
	return eg.Wait()
}

func (s *composeService) pullModel(ctx context.Context, dockerModel *manager.Plugin, model types.ModelConfig, quietPull bool, w progress.Writer) error {
	w.Event(progress.Event{
		ID:     model.Name,
		Status: progress.Working,
		Text:   "Pulling",
	})

	cmd := exec.CommandContext(ctx, dockerModel.Path, "pull", model.Model)
	s.setupChildProcess(ctx, cmd)

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
			w.Event(progress.Event{
				ID:         model.Name,
				Status:     progress.Working,
				Text:       "Pulling",
				StatusText: msg,
			})
		}
	}

	err = cmd.Wait()
	if err != nil {
		w.Event(progress.ErrorMessageEvent(model.Name, err.Error()))
	}
	w.Event(progress.Event{
		ID:     model.Name,
		Status: progress.Working,
		Text:   "Pulled",
	})
	return err
}

func (s *composeService) configureModel(ctx context.Context, dockerModel *manager.Plugin, config types.ModelConfig, w progress.Writer) error {
	w.Event(progress.Event{
		ID:     config.Name,
		Status: progress.Working,
		Text:   "Configuring",
	})
	// configure [--context-size=<n>] MODEL [-- <runtime-flags...>]
	args := []string{"configure"}
	if config.ContextSize > 0 {
		args = append(args, "--context-size", strconv.Itoa(config.ContextSize))
	}
	args = append(args, config.Model)
	if len(config.RuntimeFlags) != 0 {
		args = append(args, "--")
		args = append(args, config.RuntimeFlags...)
	}
	cmd := exec.CommandContext(ctx, dockerModel.Path, args...)
	s.setupChildProcess(ctx, cmd)
	return cmd.Run()
}

func (s *composeService) setModelVariables(ctx context.Context, dockerModel *manager.Plugin, project *types.Project) error {
	cmd := exec.CommandContext(ctx, dockerModel.Path, "status", "--json")
	s.setupChildProcess(ctx, cmd)
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
				variable = varPrefix
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

func (s *composeService) setupChildProcess(gctx context.Context, cmd *exec.Cmd) {
	// exec provider command with same environment Compose is running
	env := types.NewMapping(os.Environ())
	// but remove DOCKER_CLI_PLUGIN... variable so plugin can detect it run standalone
	delete(env, manager.ReexecEnvvar)
	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(gctx, &carrier)
	env.Merge(types.Mapping(carrier))
	env["DOCKER_CONTEXT"] = s.dockerCli.CurrentContext()
	cmd.Env = env.Values()
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
