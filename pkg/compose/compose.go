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
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/internal/projectloader"
	"github.com/docker/compose/v5/internal/registry"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/dryrun"
)

type Option func(service *composeService) error

// NewComposeService creates a Compose service using Docker CLI.
// This is the standard constructor that requires command.Cli for full functionality.
//
// Example usage:
//
//	dockerCli, _ := command.NewDockerCli()
//	service := NewComposeService(dockerCli)
//
// For advanced configuration with custom overrides, use ServiceOption functions:
//
//	service := NewComposeService(dockerCli,
//	    WithPrompt(prompt.NewPrompt(cli.In(), cli.Out()).Confirm),
//	    WithOutputStream(customOut),
//	    WithErrorStream(customErr),
//	    WithInputStream(customIn))
//
// Or set all streams at once:
//
//	service := NewComposeService(dockerCli,
//	    WithStreams(customOut, customErr, customIn))
func NewComposeService(dockerCli command.Cli, options ...Option) (api.Compose, error) {
	s := &composeService{
		dockerCli:      dockerCli,
		maxConcurrency: -1,
		dryRun:         false,

		logStreamDrainTimeout: defaultLogStreamDrainTimeout,
	}
	s.desktopAuth = true
	for _, option := range options {
		if err := option(s); err != nil {
			return nil, err
		}
	}
	if s.prompt == nil {
		s.prompt = func(message string, defaultValue bool) (bool, error) {
			fmt.Println(message)
			logrus.Warning("Compose is running without a 'prompt' component to interact with user")
			return defaultValue, nil
		}
	}
	if s.events == nil {
		s.events = progress.Noop{}
	}

	// If custom streams were provided, wrap the Docker CLI to use them
	if s.outStream != nil || s.errStream != nil || s.inStream != nil {
		s.dockerCli = s.wrapDockerCliWithStreams(dockerCli)
	}

	return s, nil
}

// WithStreams sets custom I/O streams for output and interaction
func WithStreams(out, err io.Writer, in io.Reader) Option {
	return func(s *composeService) error {
		s.outStream = out
		s.errStream = err
		s.inStream = in
		return nil
	}
}

// WithOutputStream sets a custom output stream
func WithOutputStream(out io.Writer) Option {
	return func(s *composeService) error {
		s.outStream = out
		return nil
	}
}

// WithErrorStream sets a custom error stream
func WithErrorStream(err io.Writer) Option {
	return func(s *composeService) error {
		s.errStream = err
		return nil
	}
}

// WithInputStream sets a custom input stream
func WithInputStream(in io.Reader) Option {
	return func(s *composeService) error {
		s.inStream = in
		return nil
	}
}

// WithContextInfo sets custom Docker context information
func WithContextInfo(info api.ContextInfo) Option {
	return func(s *composeService) error {
		s.contextInfo = info
		return nil
	}
}

// WithProxyConfig sets custom HTTP proxy configuration for builds
func WithProxyConfig(config map[string]string) Option {
	return func(s *composeService) error {
		s.proxyConfig = config
		return nil
	}
}

// WithPrompt configure a UI component for Compose service to interact with user and confirm actions
func WithPrompt(prompt Prompt) Option {
	return func(s *composeService) error {
		s.prompt = prompt
		return nil
	}
}

// WithMaxConcurrency defines upper limit for concurrent operations against
// engine API. A value <= 0 means unlimited.
func WithMaxConcurrency(maxConcurrency int) Option {
	return func(s *composeService) error {
		s.maxConcurrency = maxConcurrency
		return nil
	}
}

// WithDryRun configure Compose to run without actually applying changes
func WithDryRun(s *composeService) error {
	s.dryRun = true
	cli, err := command.NewDockerCli()
	if err != nil {
		return err
	}

	options := flags.NewClientOptions()
	options.Context = s.dockerCli.CurrentContext()
	err = cli.Initialize(options, command.WithInitializeClient(func(cli *command.DockerCli) (client.APIClient, error) {
		return dryrun.NewDryRunClient(s.apiClient(), s.dockerCli)
	}))
	if err != nil {
		return err
	}
	s.dockerCli = cli
	return nil
}

type Prompt func(message string, defaultValue bool) (bool, error)

// AlwaysOkPrompt returns a Prompt implementation that always returns true without user interaction.
func AlwaysOkPrompt() Prompt {
	return func(message string, defaultValue bool) (bool, error) {
		return true, nil
	}
}

// WithEventProcessor configure component to get notified on Compose operation and progress events.
// Typically used to configure a progress UI
func WithEventProcessor(bus api.EventProcessor) Option {
	return func(s *composeService) error {
		s.events = bus
		return nil
	}
}

type composeService struct {
	dockerCli command.Cli
	// prompt is used to interact with user and confirm actions
	prompt Prompt
	// navigationMenu creates the interactive menu of a foreground `up`, if any
	navigationMenu NavigationMenuFactory
	// eventBus collects tasks execution events
	events api.EventProcessor

	// Optional overrides for specific components (for SDK users)
	outStream   io.Writer
	errStream   io.Writer
	inStream    io.Reader
	contextInfo api.ContextInfo
	proxyConfig map[string]string

	maxConcurrency int
	dryRun         bool

	// desktopAuth enables Docker Desktop credentials, see authProvider.
	desktopAuth bool

	runtimeAPIVersion runtimeVersionCache

	// swarm caches the daemon's swarm mode, see isSwarmEnabled.
	swarm swarmState

	// pluginMu guards the project's services while concurrent provider runs
	// inject their variables into the services depending on them, see runPlugin.
	pluginMu sync.Mutex

	// logStreamDrainTimeout bounds the shutdown drain of re-attached log
	// streams, see drainTimeout.
	logStreamDrainTimeout time.Duration
}

// Close releases any connections/resources held by the underlying clients.
//
// In practice, this service has the same lifetime as the process, so everything
// will get cleaned up at about the same time regardless even if not invoked.
func (s *composeService) Close() error {
	var errs []error
	if s.dockerCli != nil {
		errs = append(errs, s.apiClient().Close())
	}
	return errors.Join(errs...)
}

func (s *composeService) apiClient() client.APIClient {
	return s.dockerCli.Client()
}

func (s *composeService) configFile() *configfile.ConfigFile {
	return s.dockerCli.ConfigFile()
}

// authProvider uses the config file only for services not built by
// NewComposeService, so unit tests never reach Docker Desktop.
func (s *composeService) authProvider(ctx context.Context) registry.AuthProvider {
	if !s.desktopAuth {
		return s.configFile()
	}
	return registry.NewDesktopAuthProvider(ctx, s.configFile())
}

// getContextInfo returns the context info - either custom override or dockerCli adapter
func (s *composeService) getContextInfo() api.ContextInfo {
	if s.contextInfo != nil {
		return s.contextInfo
	}
	return &dockerCliContextInfo{cli: s.dockerCli}
}

// getProxyConfig returns the proxy config - either custom override or environment-based
func (s *composeService) getProxyConfig() map[string]*string {
	if s.proxyConfig != nil {
		m := make(map[string]*string, len(s.proxyConfig))
		for k, v := range s.proxyConfig {
			m[k] = &v
		}
		return m
	}
	// proxy configuration from the CLI config file, keyed by daemon host
	// with "default" as fallback — the same lookup `docker build` uses.
	// Passing nil runOpts hands us a freshly allocated map we own.
	return s.configFile().ParseProxyConfig(s.apiClient().DaemonHost(), nil)
}

func (s *composeService) stdout() *streams.Out {
	return s.dockerCli.Out()
}

func (s *composeService) stdin() *streams.In {
	return s.dockerCli.In()
}

func (s *composeService) stderr() *streams.Out {
	return s.dockerCli.Err()
}

// readCloserAdapter adapts io.Reader to io.ReadCloser
type readCloserAdapter struct {
	r io.Reader
}

func (r *readCloserAdapter) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (r *readCloserAdapter) Close() error {
	return nil
}

// wrapDockerCliWithStreams wraps the Docker CLI to intercept and override stream methods
func (s *composeService) wrapDockerCliWithStreams(baseCli command.Cli) command.Cli {
	wrapper := &streamOverrideWrapper{
		Cli: baseCli,
	}

	// Wrap custom streams in Docker CLI's stream types
	if s.outStream != nil {
		wrapper.outStream = streams.NewOut(s.outStream)
	}
	if s.errStream != nil {
		wrapper.errStream = streams.NewOut(s.errStream)
	}
	if s.inStream != nil {
		wrapper.inStream = streams.NewIn(&readCloserAdapter{r: s.inStream})
	}

	return wrapper
}

// streamOverrideWrapper wraps command.Cli to override streams with custom implementations
type streamOverrideWrapper struct {
	command.Cli
	outStream *streams.Out
	errStream *streams.Out
	inStream  *streams.In
}

func (w *streamOverrideWrapper) Out() *streams.Out {
	if w.outStream != nil {
		return w.outStream
	}
	return w.Cli.Out()
}

func (w *streamOverrideWrapper) Err() *streams.Out {
	if w.errStream != nil {
		return w.errStream
	}
	return w.Cli.Err()
}

func (w *streamOverrideWrapper) In() *streams.In {
	if w.inStream != nil {
		return w.inStream
	}
	return w.Cli.In()
}

// projectFromName builds a types.Project based on actual resources with compose labels set
func (s *composeService) projectFromName(containers Containers, projectName string, services ...string) (*types.Project, error) {
	project := &types.Project{
		Name:     projectName,
		Services: types.Services{},
	}
	if len(containers) == 0 {
		return project, fmt.Errorf("no container found for project %q: %w", projectName, api.ErrNotFound)
	}
	set := types.Services{}
	for _, ctr := range containers {
		serviceLabel, service := projectloader.ServiceOfContainer(set, ctr)
		set[serviceLabel] = service
	}
	for name, service := range set {
		dependencies := service.Labels[api.DependenciesLabel]
		if dependencies != "" {
			service.DependsOn = decodeDependsOnLabel(dependencies)
			set[name] = service
		}
	}
	project.Services = set

SERVICES:
	for _, qs := range services {
		for _, es := range project.Services {
			if es.Name == qs {
				continue SERVICES
			}
		}
		return project, fmt.Errorf("no such service: %q: %w", qs, api.ErrNotFound)
	}
	project, err := project.WithSelectedServices(services)
	if err != nil {
		return project, err
	}

	return project, nil
}

func (s *composeService) actualVolumes(ctx context.Context, projectName string) (types.Volumes, error) {
	options := client.VolumeListOptions{
		Filters: naming.ProjectFilter(projectName),
	}
	volumes, err := s.apiClient().VolumeList(ctx, options)
	if err != nil {
		return nil, err
	}

	actual := types.Volumes{}
	for _, vol := range volumes.Items {
		actual[vol.Labels[api.VolumeLabel]] = types.VolumeConfig{
			Name:   vol.Name,
			Driver: vol.Driver,
			Labels: vol.Labels,
		}
	}
	return actual, nil
}

func (s *composeService) actualNetworks(ctx context.Context, projectName string) (types.Networks, error) {
	networks, err := s.apiClient().NetworkList(ctx, client.NetworkListOptions{
		Filters: naming.ProjectFilter(projectName),
	})
	if err != nil {
		return nil, err
	}

	actual := types.Networks{}
	for _, net := range networks.Items {
		if _, ok := net.Labels[api.RelayNetworkLabel]; ok {
			// a provider service's dedicated relay link, never a project's
			// own declared network: it carries no NetworkLabel key at all,
			// which would otherwise fold it into a bogus Networks[""] entry
			continue
		}
		actual[net.Labels[api.NetworkLabel]] = types.NetworkConfig{
			Name:   net.Name,
			Driver: net.Driver,
			Labels: net.Labels,
		}
	}
	return actual, nil
}

// swarmState caches whether the daemon this service talks to runs in swarm
// mode. The answer belongs to the daemon behind the service's client, so it is
// held per composeService rather than shared by every instance in the process.
type swarmState struct {
	once sync.Once
	val  bool
	err  error
}

func (s *composeService) isSwarmEnabled(ctx context.Context) (bool, error) {
	st := &s.swarm
	st.once.Do(func() {
		res, err := s.apiClient().Info(ctx, client.InfoOptions{})
		if err != nil {
			st.err = err
		}
		switch res.Info.Swarm.LocalNodeState {
		case swarm.LocalNodeStateInactive, swarm.LocalNodeStateLocked:
			st.val = false
		default:
			st.val = true
		}
	})
	return st.val, st.err
}

// runtimeVersionCache caches a version string after a successful lookup.
// Errors (including context cancellation) are not cached so that
// subsequent calls can retry with a fresh context.
type runtimeVersionCache struct {
	mu  sync.Mutex
	val string
}

// RuntimeAPIVersion returns the negotiated API version that will be used for
// requests to the Docker daemon. It triggers version negotiation via Ping so
// that version-gated request shaping matches the version subsequent API calls
// will actually use.
//
// After negotiation, Compose should never rely on features or request attributes
// not defined by this API version, even if the daemon's raw version is higher.
func (s *composeService) RuntimeAPIVersion(ctx context.Context) (string, error) {
	s.runtimeAPIVersion.mu.Lock()
	defer s.runtimeAPIVersion.mu.Unlock()
	if s.runtimeAPIVersion.val != "" {
		return s.runtimeAPIVersion.val, nil
	}

	cli := s.apiClient()
	_, err := cli.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true})
	if err != nil {
		return "", err
	}

	version := cli.ClientVersion()
	if version == "" {
		return "", errors.New("docker client returned empty version after successful API negotiation")
	}

	s.runtimeAPIVersion.val = version
	return s.runtimeAPIVersion.val, nil
}
