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
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type JsonMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

const (
	ErrorType  = "error"
	InfoType   = "info"
	SetEnvType = "setenv"
	DebugType  = "debug"
)

func (s *composeService) runPlugin(ctx context.Context, project *types.Project, service types.ServiceConfig, command string) error {
	provider := *service.Provider

	plugin, err := s.getPluginBinaryPath(provider.Type)
	if err != nil {
		return err
	}

	cmd := s.setupPluginCommand(ctx, project, service, plugin, command)

	variables, err := s.executePlugin(ctx, cmd, command, service)
	if err != nil {
		return err
	}

	for name, s := range project.Services {
		if _, ok := s.DependsOn[service.Name]; ok {
			prefix := strings.ToUpper(service.Name) + "_"
			for key, val := range variables {
				s.Environment[prefix+key] = &val
			}
			project.Services[name] = s
		}
	}
	return nil
}

func (s *composeService) executePlugin(ctx context.Context, cmd *exec.Cmd, command string, service types.ServiceConfig) (types.Mapping, error) {
	pw := progress.ContextWriter(ctx)
	var action string
	switch command {
	case "up":
		pw.Event(progress.CreatingEvent(service.Name))
		action = "create"
	case "down":
		pw.Event(progress.RemovingEvent(service.Name))
		action = "remove"
	default:
		return nil, fmt.Errorf("unsupported plugin command: %s", command)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(stdout)
	defer func() { _ = stdout.Close() }()

	variables := types.Mapping{}

	for {
		var msg JsonMessage
		err = decoder.Decode(&msg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch msg.Type {
		case ErrorType:
			pw.Event(progress.NewEvent(service.Name, progress.Error, msg.Message))
			return nil, errors.New(msg.Message)
		case InfoType:
			pw.Event(progress.NewEvent(service.Name, progress.Working, msg.Message))
		case SetEnvType:
			key, val, found := strings.Cut(msg.Message, "=")
			if !found {
				return nil, fmt.Errorf("invalid response from plugin: %s", msg.Message)
			}
			variables[key] = val
		case DebugType:
			logrus.Debugf("%s: %s", service.Name, msg.Message)
		default:
			return nil, fmt.Errorf("invalid response from plugin: %s", msg.Type)
		}
	}

	err = cmd.Wait()
	if err != nil {
		pw.Event(progress.ErrorMessageEvent(service.Name, err.Error()))
		return nil, fmt.Errorf("failed to %s service provider: %s", action, err.Error())
	}
	switch command {
	case "up":
		pw.Event(progress.CreatedEvent(service.Name))
	case "down":
		pw.Event(progress.RemovedEvent(service.Name))
	}
	return variables, nil
}

func (s *composeService) getPluginBinaryPath(provider string) (path string, err error) {
	if provider == "compose" {
		return "", errors.New("'compose' is not a valid provider type")
	}
	plugin, err := manager.GetPlugin(provider, s.dockerCli, &cobra.Command{})
	if err == nil {
		path = plugin.Path
	}
	if manager.IsNotFound(err) {
		path, err = exec.LookPath(provider)
	}
	return path, err
}

func (s *composeService) setupPluginCommand(ctx context.Context, project *types.Project, service types.ServiceConfig, path, command string) *exec.Cmd {
	provider := *service.Provider

	args := []string{"compose", "--project-name", project.Name, command}
	for k, v := range provider.Options {
		for _, value := range v {
			args = append(args, fmt.Sprintf("--%s=%s", k, value))
		}
	}
	args = append(args, service.Name)

	cmd := exec.CommandContext(ctx, path, args...)
	// exec provider command with same environment Compose is running
	env := types.NewMapping(os.Environ())
	// but remove DOCKER_CLI_PLUGIN... variable so plugin can detect it run standalone
	delete(env, manager.ReexecEnvvar)
	// and add the explicit environment variables set for service
	for key, val := range service.Environment.RemoveEmpty().ToMapping() {
		env[key] = val
	}
	cmd.Env = env.Values()

	// Use docker/cli mechanism to propagate termination signal to child process
	server, err := socket.NewPluginServer(nil)
	if err == nil {
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
