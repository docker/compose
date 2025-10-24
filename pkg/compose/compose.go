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
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/buildx/store/storeutil"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v2/pkg/api"
)

var stdioToStdout bool

func init() {
	out, ok := os.LookupEnv("COMPOSE_STATUS_STDOUT")
	if ok {
		stdioToStdout, _ = strconv.ParseBool(out)
	}
}

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
		clock:          clockwork.NewRealClock(),
		maxConcurrency: -1,
		dryRun:         false,
		events: func(ctx context.Context, e ...progress.Event) {
			// FIXME(ndeloof) temporary during refactoring
			progress.ContextWriter(ctx).Events(e)
		},
	}
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

// WithMaxConcurrency defines upper limit for concurrent operations against engine API
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
		return api.NewDryRunClient(s.apiClient(), s.dockerCli)
	}))
	if err != nil {
		return err
	}
	s.dockerCli = cli
	return nil
}

type Prompt func(message string, defaultValue bool) (bool, error)

type EventBus func(ctx context.Context, e ...progress.Event)

type composeService struct {
	dockerCli command.Cli
	// prompt is used to interact with user and confirm actions
	prompt Prompt
	// eventBus collects tasks execution events
	events EventBus

	// Optional overrides for specific components (for SDK users)
	outStream   io.Writer
	errStream   io.Writer
	inStream    io.Reader
	contextInfo api.ContextInfo
	proxyConfig map[string]string

	clock          clockwork.Clock
	maxConcurrency int
	dryRun         bool
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

// getContextInfo returns the context info - either custom override or dockerCli adapter
func (s *composeService) getContextInfo() api.ContextInfo {
	if s.contextInfo != nil {
		return s.contextInfo
	}
	return &dockerCliContextInfo{cli: s.dockerCli}
}

// getProxyConfig returns the proxy config - either custom override or environment-based
func (s *composeService) getProxyConfig() map[string]string {
	if s.proxyConfig != nil {
		return s.proxyConfig
	}
	return storeutil.GetProxyConfig(s.dockerCli)
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

func (s *composeService) stdinfo() *streams.Out {
	if stdioToStdout {
		return s.stdout()
	}
	return s.stderr()
}

// GetConfiguredStreams returns the configured I/O streams (implements api.Compose interface)
func (s *composeService) GetConfiguredStreams() (io.Writer, io.Writer, io.Reader) {
	return s.stdout(), s.stderr(), s.stdin()
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

func getCanonicalContainerName(c container.Summary) string {
	if len(c.Names) == 0 {
		// corner case, sometime happens on removal. return short ID as a safeguard value
		return c.ID[:12]
	}
	// Names return container canonical name /foo  + link aliases /linked_by/foo
	for _, name := range c.Names {
		if strings.LastIndex(name, "/") == 0 {
			return name[1:]
		}
	}

	return strings.TrimPrefix(c.Names[0], "/")
}

func getContainerNameWithoutProject(c container.Summary) string {
	project := c.Labels[api.ProjectLabel]
	defaultName := getDefaultContainerName(project, c.Labels[api.ServiceLabel], c.Labels[api.ContainerNumberLabel])
	name := getCanonicalContainerName(c)
	if name != defaultName {
		// service declares a custom container_name
		return name
	}
	return name[len(project)+1:]
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
	for _, c := range containers {
		serviceLabel, ok := c.Labels[api.ServiceLabel]
		if !ok {
			serviceLabel = getCanonicalContainerName(c)
		}
		service, ok := set[serviceLabel]
		if !ok {
			service = types.ServiceConfig{
				Name:   serviceLabel,
				Image:  c.Image,
				Labels: c.Labels,
			}
		}
		service.Scale = increment(service.Scale)
		set[serviceLabel] = service
	}
	for name, service := range set {
		dependencies := service.Labels[api.DependenciesLabel]
		if dependencies != "" {
			service.DependsOn = types.DependsOnConfig{}
			for _, dc := range strings.Split(dependencies, ",") {
				dcArr := strings.Split(dc, ":")
				condition := ServiceConditionRunningOrHealthy
				// Let's restart the dependency by default if we don't have the info stored in the label
				restart := true
				required := true
				dependency := dcArr[0]

				// backward compatibility
				if len(dcArr) > 1 {
					condition = dcArr[1]
					if len(dcArr) > 2 {
						restart, _ = strconv.ParseBool(dcArr[2])
					}
				}
				service.DependsOn[dependency] = types.ServiceDependency{Condition: condition, Restart: restart, Required: required}
			}
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

func increment(scale *int) *int {
	i := 1
	if scale != nil {
		i = *scale + 1
	}
	return &i
}

func (s *composeService) actualVolumes(ctx context.Context, projectName string) (types.Volumes, error) {
	opts := volume.ListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
	}
	volumes, err := s.apiClient().VolumeList(ctx, opts)
	if err != nil {
		return nil, err
	}

	actual := types.Volumes{}
	for _, vol := range volumes.Volumes {
		actual[vol.Labels[api.VolumeLabel]] = types.VolumeConfig{
			Name:   vol.Name,
			Driver: vol.Driver,
			Labels: vol.Labels,
		}
	}
	return actual, nil
}

func (s *composeService) actualNetworks(ctx context.Context, projectName string) (types.Networks, error) {
	networks, err := s.apiClient().NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
	})
	if err != nil {
		return nil, err
	}

	actual := types.Networks{}
	for _, net := range networks {
		actual[net.Labels[api.NetworkLabel]] = types.NetworkConfig{
			Name:   net.Name,
			Driver: net.Driver,
			Labels: net.Labels,
		}
	}
	return actual, nil
}

var swarmEnabled = struct {
	once sync.Once
	val  bool
	err  error
}{}

func (s *composeService) isSWarmEnabled(ctx context.Context) (bool, error) {
	swarmEnabled.once.Do(func() {
		info, err := s.apiClient().Info(ctx)
		if err != nil {
			swarmEnabled.err = err
		}
		switch info.Swarm.LocalNodeState {
		case swarm.LocalNodeStateInactive, swarm.LocalNodeStateLocked:
			swarmEnabled.val = false
		default:
			swarmEnabled.val = true
		}
	})
	return swarmEnabled.val, swarmEnabled.err
}

type runtimeVersionCache struct {
	once sync.Once
	val  string
	err  error
}

var runtimeVersion runtimeVersionCache

func (s *composeService) RuntimeVersion(ctx context.Context) (string, error) {
	runtimeVersion.once.Do(func() {
		version, err := s.apiClient().ServerVersion(ctx)
		if err != nil {
			runtimeVersion.err = err
		}
		runtimeVersion.val = version.APIVersion
	})
	return runtimeVersion.val, runtimeVersion.err
}
