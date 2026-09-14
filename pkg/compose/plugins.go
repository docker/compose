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
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/v29/cli-plugins/manager"
	"github.com/docker/cli/v29/cli/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v5/pkg/api"
)

type JsonMessage struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

const (
	ErrorType                 = "error"
	InfoType                  = "info"
	SetEnvType                = "setenv"
	RawSetEnvType             = "rawsetenv"
	DebugType                 = "debug"
	providerMetadataDirectory = "compose/providers"

	// GetServiceConfigType is a message the provider sends to receive, on
	// its stdin, one JSON line holding the resolved canonical configuration
	// of the service it manages — answered from the in-memory model.
	GetServiceConfigType = "get-service-config"
)

type pluginVariables struct {
	prefixed types.Mapping
	raw      types.Mapping
}

var mux sync.Mutex

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
	if cmd == nil {
		return nil
	}

	variables, err := s.executePlugin(cmd, command, service)
	if err != nil {
		return err
	}

	if command == "stop" {
		return nil
	}

	mux.Lock()
	defer mux.Unlock()
	for name, s := range project.Services {
		if _, ok := s.DependsOn[service.Name]; ok {
			prefix := strings.ToUpper(service.Name) + "_"
			for key, val := range variables.prefixed {
				s.Environment[prefix+key] = &val
			}
			for key, val := range variables.raw {
				if existing, ok := s.Environment[key]; ok && (existing == nil || *existing != val) {
					logrus.Warnf("provider %q overrides environment variable %q in service %q", service.Name, key, name)
				}
				s.Environment[key] = &val
			}
			project.Services[name] = s
		}
	}
	return nil
}

func (s *composeService) executePlugin(cmd *exec.Cmd, command string, service types.ServiceConfig) (pluginVariables, error) {
	var action string
	switch command {
	case "up":
		s.events.On(creatingEvent(service.Name))
		action = "create"
	case "down":
		s.events.On(removingEvent(service.Name))
		action = "remove"
	case "stop":
		s.events.On(stoppingEvent(service.Name))
		action = "stop"
	default:
		return pluginVariables{}, fmt.Errorf("unsupported plugin command: %s", command)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return pluginVariables{}, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return pluginVariables{}, err
	}
	// Answers are written from their own goroutine: writing a config larger
	// than the OS pipe buffer from the read loop would deadlock against a
	// provider that emits stdout before draining its stdin. Every answer is
	// the same serialized service, so completion order is irrelevant; the
	// mutex keeps individual writes atomic. Write failures are not reported
	// from here — a provider missing its answer reads EOF once stdin closes.
	var stdinMu sync.Mutex
	var answers sync.WaitGroup
	processExited := false
	// Closing stdin on exit unblocks a provider waiting for a response the
	// loop will never produce (e.g. a request emitted after an error). An
	// answer dispatched but not yet written must land before the close —
	// but only once the process has exited can a write not block forever
	// (a dead peer turns it into EPIPE); on error paths the provider may
	// still be alive and not reading, so close first to error the write
	// out instead of hanging the wait.
	defer func() {
		if processExited {
			answers.Wait()
			_ = stdin.Close()
		} else {
			_ = stdin.Close()
			answers.Wait()
		}
	}()

	err = cmd.Start()
	if err != nil {
		return pluginVariables{}, err
	}
	// Error paths return before the normal cmd.Wait below and would leave
	// the provider as a zombie (and possibly running): reap it — kill
	// first, as it may be misbehaving or blocked, which also errors out
	// any in-flight answer write. Runs before the stdin/answers defer
	// above (LIFO).
	defer func() {
		if !processExited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	decoder := json.NewDecoder(stdout)
	defer func() { _ = stdout.Close() }()

	variables := pluginVariables{
		prefixed: types.Mapping{},
		raw:      types.Mapping{},
	}

	for {
		var msg JsonMessage
		err = decoder.Decode(&msg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return pluginVariables{}, err
		}
		switch msg.Type {
		case ErrorType:
			s.events.On(newEvent(service.Name, api.Error, firstLine(msg.Message)))
			return pluginVariables{}, errors.New(msg.Message)
		case InfoType:
			s.events.On(newEvent(service.Name, api.Working, firstLine(msg.Message)))
		case SetEnvType:
			key, val, found := strings.Cut(msg.Message, "=")
			if !found {
				return pluginVariables{}, fmt.Errorf("invalid response from plugin: %s", msg.Message)
			}
			variables.prefixed[key] = val
		case RawSetEnvType:
			key, val, found := strings.Cut(msg.Message, "=")
			if !found {
				return pluginVariables{}, fmt.Errorf("invalid response from plugin: %s", msg.Message)
			}
			variables.raw[key] = val
		case GetServiceConfigType:
			payload, err := json.Marshal(service)
			if err != nil {
				return pluginVariables{}, fmt.Errorf("failed to answer get-service-config: %w", err)
			}
			payload = append(payload, '\n')
			answers.Add(1)
			go func() {
				defer answers.Done()
				stdinMu.Lock()
				defer stdinMu.Unlock()
				_, _ = stdin.Write(payload)
			}()
		case DebugType:
			logrus.Debugf("%s: %s", service.Name, msg.Message)
		default:
			return pluginVariables{}, fmt.Errorf("invalid response from plugin: %s", msg.Type)
		}
	}

	err = cmd.Wait()
	processExited = true
	if err != nil {
		s.events.On(errorEvent(service.Name, err.Error()))
		return pluginVariables{}, fmt.Errorf("failed to %s service provider: %s", action, err.Error())
	}
	switch command {
	case "up":
		s.events.On(createdEvent(service.Name))
	case "down":
		s.events.On(removedEvent(service.Name))
	case "stop":
		s.events.On(stoppedEvent(service.Name))
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
	if errdefs.IsNotFound(err) {
		// A plain LookPath honors Windows executable-resolution semantics
		// (PATHEXT: .exe, but also .com/.bat/.cmd), so provider executables
		// need not be compiled binaries. Callers must not append a hardcoded
		// ".exe": it restricts the lookup instead of helping it.
		path, err = exec.LookPath(provider)
	}
	return path, err
}

func (s *composeService) setupPluginCommand(ctx context.Context, project *types.Project, service types.ServiceConfig, path, command string) (*exec.Cmd, error) {
	cmdOptionsMetadata := s.getPluginMetadata(path, service.Provider.Type, project)
	var currentCommandMetadata CommandMetadata
	switch command {
	case "up":
		currentCommandMetadata = cmdOptionsMetadata.Up
	case "down":
		currentCommandMetadata = cmdOptionsMetadata.Down
	case "stop":
		if cmdOptionsMetadata.Stop == nil {
			return nil, nil
		}
		currentCommandMetadata = *cmdOptionsMetadata.Stop
	}

	provider := *service.Provider
	commandMetadataIsEmpty := cmdOptionsMetadata.IsEmpty()
	if err := currentCommandMetadata.CheckRequiredParameters(provider); !commandMetadataIsEmpty && err != nil {
		return nil, err
	}

	args := []string{"compose", "--project-name=" + project.Name, command}
	for k, v := range provider.Options {
		for _, value := range v {
			if _, ok := currentCommandMetadata.GetParameter(k); commandMetadataIsEmpty || ok {
				args = append(args, fmt.Sprintf("--%s=%s", k, value))
			}
		}
	}
	args = append(args, service.Name)

	cmd := exec.CommandContext(ctx, path, args...)

	err := s.prepareShellOut(ctx, project.Environment, cmd)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func (s *composeService) getPluginMetadata(path, command string, project *types.Project) ProviderMetadata {
	cmd := exec.Command(path, "compose", "metadata")
	err := s.prepareShellOut(context.Background(), project.Environment, cmd)
	if err != nil {
		logrus.Debugf("failed to prepare plugin metadata command: %v", err)
		return ProviderMetadata{}
	}
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
	Description string           `json:"description"`
	Up          CommandMetadata  `json:"up"`
	Down        CommandMetadata  `json:"down"`
	Stop        *CommandMetadata `json:"stop,omitempty"`
}

func (p ProviderMetadata) IsEmpty() bool {
	return p.Description == "" && p.Up.Parameters == nil && p.Down.Parameters == nil
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

// firstLine returns the first line of s, stripping any trailing newlines.
func firstLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}
