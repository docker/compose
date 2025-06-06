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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/socket"
	"github.com/docker/cli/cli/config"
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
	ErrorType                 = "error"
	InfoType                  = "info"
	SetEnvType                = "setenv"
	DebugType                 = "debug"
	providerMetadataDirectory = "compose/providers"
)

func (s *composeService) runPlugin(ctx context.Context, project *types.Project, service types.ServiceConfig, command string) error {
	provider := *service.Provider

	plugin, err := s.getPluginBinaryPath(provider.Type)
	if err != nil {
		return err
	}

	cmd, err := s.setupPluginCommand(ctx, project, service, plugin, command)
	if err != nil {
		return err
	}

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
		path, err = exec.LookPath(executable(provider))
	}
	return path, err
}

func (s *composeService) setupPluginCommand(ctx context.Context, project *types.Project, service types.ServiceConfig, path, command string) (*exec.Cmd, error) {
	cmdOptionsMetadata := s.getPluginMetadata(path, service.Provider.Type)
	var currentCommandMetadata CommandMetadata
	switch command {
	case "up":
		currentCommandMetadata = cmdOptionsMetadata.Up
	case "down":
		currentCommandMetadata = cmdOptionsMetadata.Down
	}
	commandMetadataIsEmpty := len(currentCommandMetadata.Parameters) == 0
	provider := *service.Provider
	if err := currentCommandMetadata.CheckRequiredParameters(provider); !commandMetadataIsEmpty && err != nil {
		return nil, err
	}

	args := []string{"compose", "--project-name", project.Name, command}
	for k, v := range provider.Options {
		for _, value := range v {
			if _, ok := currentCommandMetadata.GetParameter(k); commandMetadataIsEmpty || ok {
				args = append(args, fmt.Sprintf("--%s=%s", k, value))
			}
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
		cmd.Env = replace(cmd.Env, socket.EnvKey, server.Addr().String())
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_CONTEXT=%s", s.dockerCli.CurrentContext()))

	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, &carrier)
	cmd.Env = append(cmd.Env, types.Mapping(carrier).Values()...)
	return cmd, nil
}

func (s *composeService) getPluginMetadata(path, command string) ProviderMetadata {
	cmd := exec.Command(path, "compose", "metadata")
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	if err := cmd.Run(); err != nil {
		logrus.Debugf("failed to start plugin metadata command: %v", err)
		return ProviderMetadata{}
	}

	var metadata ProviderMetadata
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
		output, _ := io.ReadAll(stdout)
		logrus.Debugf("failed to decode plugin metadata: %v - %s", err, output)
		return ProviderMetadata{}
	}
	// Save metadata into docker home directory to be used by Docker LSP tool
	// Just log the error as it's not a critical error for the main flow
	metadataDir := filepath.Join(config.Dir(), providerMetadataDirectory)
	if err := os.MkdirAll(metadataDir, 0o700); err == nil {
		metadataFilePath := filepath.Join(metadataDir, command+".json")
		if err := os.WriteFile(metadataFilePath, stdout.Bytes(), 0o600); err != nil {
			logrus.Debugf("failed to save plugin metadata: %v", err)
		}
	} else {
		logrus.Debugf("failed to create plugin metadata directory: %v", err)
	}
	return metadata
}

type ProviderMetadata struct {
	Description string          `json:"description"`
	Up          CommandMetadata `json:"up"`
	Down        CommandMetadata `json:"down"`
}

type CommandMetadata struct {
	Parameters []ParameterMetadata `json:"parameters"`
}

type ParameterMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}

func (c CommandMetadata) GetParameter(paramName string) (ParameterMetadata, bool) {
	for _, p := range c.Parameters {
		if p.Name == paramName {
			return p, true
		}
	}
	return ParameterMetadata{}, false
}

func (c CommandMetadata) CheckRequiredParameters(provider types.ServiceProviderConfig) error {
	for _, p := range c.Parameters {
		if p.Required {
			if _, ok := provider.Options[p.Name]; !ok {
				return fmt.Errorf("required parameter %q is missing from provider %q definition", p.Name, provider.Type)
			}
		}
	}
	return nil
}
