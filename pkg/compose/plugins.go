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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/socket"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
)

type JsonMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

const (
	ErrorType  = "error"
	InfoType   = "info"
	SetEnvType = "setenv"
)

func (s *composeService) runPlugin(ctx context.Context, project *types.Project, service types.ServiceConfig, command string) error {
	provider := *service.Provider

	path, err := s.getPluginBinaryPath(provider.Type)
	if err != nil {
		return err
	}

	cmd := s.setupPluginCommand(ctx, project, provider, path, command)

	eg := errgroup.Group{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}
	eg.Go(cmd.Wait)

	decoder := json.NewDecoder(stdout)
	defer func() { _ = stdout.Close() }()

	variables := types.Mapping{}

	pw := progress.ContextWriter(ctx)
	pw.Event(progress.CreatingEvent(service.Name))
	for {
		var msg JsonMessage
		err = decoder.Decode(&msg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch msg.Type {
		case ErrorType:
			pw.Event(progress.ErrorMessageEvent(service.Name, "error"))
			return errors.New(msg.Message)
		case InfoType:
			pw.Event(progress.ErrorMessageEvent(service.Name, msg.Message))
		case SetEnvType:
			key, val, found := strings.Cut(msg.Message, "=")
			if !found {
				return fmt.Errorf("invalid response from plugin: %s", msg.Message)
			}
			variables[key] = val
		default:
			return fmt.Errorf("invalid response from plugin: %s", msg.Type)
		}
	}

	err = eg.Wait()
	if err != nil {
		pw.Event(progress.ErrorMessageEvent(service.Name, err.Error()))
		return fmt.Errorf("failed to create external service: %s", err.Error())
	}
	pw.Event(progress.CreatedEvent(service.Name))

	prefix := strings.ToUpper(service.Name) + "_"
	for name, s := range project.Services {
		if _, ok := s.DependsOn[service.Name]; ok {
			for key, val := range variables {
				s.Environment[prefix+key] = &val
			}
			project.Services[name] = s
		}
	}
	return nil
}

func (s *composeService) getPluginBinaryPath(providerType string) (string, error) {
	// Only support Docker CLI plugins for first iteration. Could support any binary from PATH
	plugin, err := manager.GetPlugin(providerType, s.dockerCli, &cobra.Command{})
	if err != nil {
		return "", err
	}
	return plugin.Path, nil
}

func (s *composeService) setupPluginCommand(ctx context.Context, project *types.Project, provider types.ServiceProviderConfig, path, command string) *exec.Cmd {
	args := []string{"compose", "--project-name", project.Name, command}
	for k, v := range provider.Options {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}

	cmd := exec.CommandContext(ctx, path, args...)
	// Remove DOCKER_CLI_PLUGIN... variable so plugin can detect it run standalone
	cmd.Env = filter(os.Environ(), manager.ReexecEnvvar)

	// Use docker/cli mechanism to propagate termination signal to child process
	server, err := socket.NewPluginServer(nil)
	if err != nil {
		defer server.Close() //nolint:errcheck
		cmd.Cancel = server.Close
		cmd.Env = replace(cmd.Env, socket.EnvKey, server.Addr().String())
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_CONTEXT=%s", s.dockerCli.CurrentContext()))

	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, &carrier)
	cmd.Env = append(cmd.Env, types.Mapping(carrier).Values()...)
	return cmd
}
