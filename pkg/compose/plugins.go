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
	"net"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/config"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v5/pkg/api"
)

type JsonMessage struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	// Platform optionally narrows a get-image request to one platform of a
	// multi-platform image (e.g. "linux/arm64").
	Platform string `json:"platform,omitempty"`
}

const (
	ErrorType                 = "error"
	InfoType                  = "info"
	SetEnvType                = "setenv"
	RawSetEnvType             = "rawsetenv"
	DebugType                 = "debug"
	PublishEndpointType       = "publish-endpoint"
	providerMetadataDirectory = "compose/providers"

	// GetServiceConfigType is a message the provider sends to receive, on
	// its stdin, one JSON line holding the resolved canonical configuration
	// of the service it manages — answered from the in-memory model.
	GetServiceConfigType = "get-service-config"

	// GetRelayInfoType is a message the provider sends to receive, on its
	// stdin, one JSON line describing the project networks the relay
	// standing in for this service would join, with each network's gateway
	// address. Only meaningful for a provider running its service LOCALLY:
	// the gateway is an address the host owns on the network's bridge, so
	// an endpoint bound to it is reachable from the relay without being
	// exposed on the LAN. A provider backing the service with a remote
	// resource has no use for it.
	GetRelayInfoType = "get-relay-info"

	// GetImageType is a message the provider sends during its pull command
	// to receive the service image from the local daemon: compose answers on
	// the provider's stdin with one ImageStreamType JSON line, then — unless
	// that line carries an error — the image tar encoded as an HTTP/1.1
	// chunked body (see streamImageTo).
	GetImageType = "get-image"

	// ImageStreamType is the type of the JSON line answering a get-image
	// request.
	ImageStreamType = "image-stream"

	// ComposeProviderMessagesEnv announces to the provider process, as a
	// comma-separated list, every message type this compose accepts on the
	// provider's stdout — so a provider can adapt to the compose it runs
	// under instead of emitting a message that would fail the command
	// (an unknown message type is a protocol error by design: a provider
	// REQUIRING an unsupported message must fail rather than degrade
	// silently).
	ComposeProviderMessagesEnv = "COMPOSE_PROVIDER_MESSAGES"
)

// providerMessageTypes is the COMPOSE_PROVIDER_MESSAGES value: every message
// type executePlugin's loop accepts. Keep in sync with its switch.
var providerMessageTypes = strings.Join([]string{
	ErrorType,
	InfoType,
	SetEnvType,
	RawSetEnvType,
	DebugType,
	PublishEndpointType,
	GetServiceConfigType,
	GetRelayInfoType,
	GetImageType,
}, ",")

// imageStreamAnswer is the stdin answer to a get-image request. On success
// Encoding and MediaType describe the byte stream that follows the JSON line;
// on failure Error carries the reason and no stream follows.
type imageStreamAnswer struct {
	Type      string `json:"type"`
	Encoding  string `json:"encoding,omitempty"`
	MediaType string `json:"media-type,omitempty"`
	Error     string `json:"error,omitempty"`
}

type pluginVariables struct {
	prefixed types.Mapping
	raw      types.Mapping
	// endpoints are "port=host:port" publish-endpoint messages: container
	// port the consumers know, mapped to where the provider's resource
	// actually listens. When present, compose deploys a relay container
	// under the service's name on the consumers' networks.
	endpoints map[int]string
}

var mux sync.Mutex

func (s *composeService) runPlugin(ctx context.Context, project *types.Project, service types.ServiceConfig, command string, extraArgs ...string) error {
	provider := *service.Provider

	plugin, err := s.getPluginBinaryPath(provider.Type)
	if err != nil {
		return err
	}

	cmd, err := s.setupPluginCommand(ctx, project, service, plugin, command, extraArgs...)
	if err != nil {
		return err
	}
	if cmd == nil {
		return nil
	}

	variables, err := s.executePlugin(ctx, project, cmd, command, service)
	if err != nil {
		return err
	}

	if command == "stop" || command == "pull" {
		return nil
	}

	isUp := command == "up"
	deployRelay := isUp && len(variables.endpoints) > 0

	// project.Services is shared state mutated by every concurrent provider
	// run: the env-var injection below writes it, and the relay's network
	// selection reads it — both belong under the mutex. The Docker API work
	// in ensureServiceRelay does not: holding the lock across it would make
	// every concurrent provider wait on the slowest one (image pull
	// included), so the relay is deployed after the lock is released.
	mux.Lock()
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
	var networkKeys []string
	if deployRelay {
		networkKeys = relayNetworks(project, service)
	}
	mux.Unlock()

	if isUp {
		if deployRelay {
			return s.ensureServiceRelay(ctx, project, service, variables.endpoints, networkKeys)
		}
		// The provider published no endpoint on this run — whether it never
		// did, or stopped doing so since a previous up deployed a relay for
		// it. Either way there is nothing to route to, so any relay left
		// over from an earlier run must go: routing to whatever upstream it
		// still holds would be silently wrong instead of just absent.
		return s.removeServiceRelay(ctx, project.Name, service.Name)
	}
	return nil
}

// parseEndpointMessage decodes a publish-endpoint payload: "80=host:port".
func parseEndpointMessage(message string) (int, string, error) {
	portPart, upstream, found := strings.Cut(message, "=")
	if !found {
		return 0, "", fmt.Errorf("publish-endpoint %q: want port=host:port", message)
	}
	port, err := strconv.Atoi(portPart)
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("publish-endpoint %q: invalid port", message)
	}
	if _, _, err := net.SplitHostPort(upstream); err != nil {
		return 0, "", fmt.Errorf("publish-endpoint %q: invalid endpoint: %w", message, err)
	}
	return port, upstream, nil
}

func (s *composeService) executePlugin(ctx context.Context, project *types.Project, cmd *exec.Cmd, command string, service types.ServiceConfig) (pluginVariables, error) {
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
	case "pull":
		s.events.On(newEvent(service.Name, api.Working, "Pulling"))
		action = "pull"
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
		prefixed:  types.Mapping{},
		raw:       types.Mapping{},
		endpoints: map[int]string{},
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
		if err := s.handlePluginMessage(ctx, project, service, msg, command, &variables, stdin, &stdinMu, &answers); err != nil {
			return pluginVariables{}, err
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
	case "pull":
		s.events.On(newEvent(service.Name, api.Done, "Pulled"))
	}
	return variables, nil
}

// imageStreamChunkSize is the fixed buffer compose fills from the image
// export before flushing it as one chunk of the stream.
const imageStreamChunkSize = 1024 * 1024

// streamImageTo answers a get-image request on the provider's stdin: one
// ImageStreamType JSON line announcing the transfer, then the image tar
// encoded as HTTP/1.1 chunked data (RFC 9112 §7.1) — every block of data
// prefixed by its length, the zero-length chunk marking a COMPLETE
// transfer. Length-prefixed framing needs no in-band delimiter (any
// byte value can appear inside a tar) and is consumable with any language's
// chunked-body reader. A stream that ends without the terminating zero chunk
// was aborted: the provider must discard it.
// When the image cannot be exported, the announce line carries an error
// instead and no stream follows.
func (s *composeService) streamImageTo(ctx context.Context, w io.Writer, ref, platform string) error {
	announce := func(a imageStreamAnswer) error {
		payload, err := json.Marshal(a)
		if err != nil {
			return err
		}
		_, err = w.Write(append(payload, '\n'))
		return err
	}

	var opts []client.ImageSaveOption
	if platform != "" {
		p, err := platforms.Parse(platform)
		if err != nil {
			_ = announce(imageStreamAnswer{Type: ImageStreamType, Error: fmt.Sprintf("invalid platform %q: %s", platform, err)})
			return err
		}
		opts = append(opts, client.ImageSaveWithPlatforms(p))
	}
	tar, err := s.apiClient().ImageSave(ctx, []string{ref}, opts...)
	if err != nil {
		_ = announce(imageStreamAnswer{Type: ImageStreamType, Error: err.Error()})
		return err
	}
	defer func() { _ = tar.Close() }()

	if err := announce(imageStreamAnswer{Type: ImageStreamType, Encoding: "chunked", MediaType: "application/x-tar"}); err != nil {
		return err
	}
	cw := httputil.NewChunkedWriter(w)
	if _, err := io.CopyBuffer(cw, struct{ io.Reader }{tar}, make([]byte, imageStreamChunkSize)); err != nil {
		return err
	}
	// ChunkedWriter.Close writes the zero-length chunk, which ends the
	// stream. Unlike an HTTP message there is deliberately no trailer
	// section nor final CRLF: a stock chunked reader stops at the zero
	// chunk without consuming either, and leftover bytes would corrupt the
	// next JSON answer a provider requesting several images reads.
	return cw.Close()
}

// handlePluginMessage processes one provider message, mutating variables in
// place; a returned error is terminal for the run (the provider is killed
// and the command fails). An unknown message type IS such an error: a
// provider requiring a message this compose does not support must fail
// loudly, not degrade silently — providers adapt through the
// COMPOSE_PROVIDER_MESSAGES announcement instead. Answer-bearing requests
// (get-service-config, get-relay-info, get-image) are served from their own
// goroutine — see the answers/stdinMu contract in executePlugin.
func (s *composeService) handlePluginMessage(
	ctx context.Context, project *types.Project, service types.ServiceConfig, msg JsonMessage, command string,
	variables *pluginVariables, stdin io.WriteCloser, stdinMu *sync.Mutex, answers *sync.WaitGroup,
) error {
	switch msg.Type {
	case ErrorType:
		s.events.On(newEvent(service.Name, api.Error, firstLine(msg.Message)))
		return errors.New(msg.Message)
	case InfoType:
		s.events.On(newEvent(service.Name, api.Working, firstLine(msg.Message)))
	case SetEnvType:
		key, val, found := strings.Cut(msg.Message, "=")
		if !found {
			return fmt.Errorf("invalid message from plugin: %s", msg.Message)
		}
		variables.prefixed[key] = val
	case RawSetEnvType:
		key, val, found := strings.Cut(msg.Message, "=")
		if !found {
			return fmt.Errorf("invalid message from plugin: %s", msg.Message)
		}
		variables.raw[key] = val
	case GetServiceConfigType:
		payload, err := json.Marshal(service)
		if err != nil {
			return fmt.Errorf("failed to answer get-service-config: %w", err)
		}
		answerProvider(stdin, stdinMu, answers, payload)
	case GetRelayInfoType:
		// resolved lazily, on request only: the network inspects cost
		// nothing to providers that never ask (remote-resource providers),
		// and the networks exist by the time an up runs
		payload, err := json.Marshal(s.relayInfo(ctx, project, service))
		if err != nil {
			return fmt.Errorf("failed to answer get-relay-info: %w", err)
		}
		answerProvider(stdin, stdinMu, answers, payload)
	case GetImageType:
		// image distribution belongs to the pull command: answering it
		// elsewhere would let an image export stall a down or a stop
		if command != "pull" {
			return fmt.Errorf("invalid message from plugin: %s is only supported during the pull command", GetImageType)
		}
		ref, platform := msg.Message, msg.Platform
		answers.Add(1)
		go func() {
			defer answers.Done()
			// stdinMu is deliberately held for the whole transfer: the
			// chunked body must be contiguous on stdin, any concurrent
			// answer interleaved into it would corrupt the framing. A
			// provider must therefore drain the announced stream before
			// expecting any other answer (documented contract); compose
			// cannot hang forever on a provider that stops reading — the
			// error path kills the process, which EPIPEs the write.
			stdinMu.Lock()
			defer stdinMu.Unlock()
			if err := s.streamImageTo(ctx, stdin, ref, platform); err != nil {
				logrus.Warnf("provider %q: get-image %q: %v", service.Name, ref, err)
				// A failure after the success announce leaves the stream
				// without its terminating chunk, and the channel cannot be
				// resynchronized (any byte would read as chunk data).
				// Closing stdin makes the truncation observable — the
				// provider gets EOF mid-chunk and discards, instead of
				// blocking forever on a stream nobody will finish.
				_ = stdin.Close()
			}
		}()
	case PublishEndpointType:
		port, upstream, err := parseEndpointMessage(msg.Message)
		if err != nil {
			return fmt.Errorf("invalid message from plugin: %w", err)
		}
		variables.endpoints[port] = upstream
	case DebugType:
		logrus.Debugf("%s: %s", service.Name, msg.Message)
	default:
		return fmt.Errorf("invalid message from plugin: %s", msg.Type)
	}
	return nil
}

// answerProvider dispatches one JSON-line answer to the provider's stdin
// from its own goroutine: writing a payload larger than the OS pipe buffer
// from the read loop would deadlock against a provider that emits stdout
// before draining its stdin. The mutex keeps individual writes atomic; a
// write failure is not reported — a provider missing its answer reads EOF
// once stdin closes.
func answerProvider(stdin io.WriteCloser, stdinMu *sync.Mutex, answers *sync.WaitGroup, payload []byte) {
	payload = append(payload, '\n')
	answers.Add(1)
	go func() {
		defer answers.Done()
		stdinMu.Lock()
		defer stdinMu.Unlock()
		_, _ = stdin.Write(payload)
	}()
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

func (s *composeService) setupPluginCommand(ctx context.Context, project *types.Project, service types.ServiceConfig, path, command string, extraArgs ...string) (*exec.Cmd, error) {
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
	case "pull":
		// image distribution is opt-in, declared like stop by the presence
		// of the command block in the provider metadata
		if cmdOptionsMetadata.Pull == nil {
			return nil, nil
		}
		currentCommandMetadata = *cmdOptionsMetadata.Pull
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
	args = append(args, extraArgs...)
	args = append(args, service.Name)

	cmd := exec.CommandContext(ctx, path, args...)

	err := s.prepareShellOut(ctx, project.Environment, cmd)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Env, ComposeProviderMessagesEnv+"="+providerMessageTypes)
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
	// Pull declares support for the image-distribution command; like Stop,
	// the presence of the block is what opts the provider in.
	Pull *CommandMetadata `json:"pull,omitempty"`
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
