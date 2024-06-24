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
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/compose/v2/internal/desktop"
	"github.com/docker/compose/v2/internal/experimental"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/jonboulle/clockwork"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/api"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

var stdioToStdout bool

func init() {
	out, ok := os.LookupEnv("COMPOSE_STATUS_STDOUT")
	if ok {
		stdioToStdout, _ = strconv.ParseBool(out)
	}
}

// NewComposeService create a local implementation of the compose.Service API
func NewComposeService(dockerCli command.Cli) api.Service {
	return &composeService{
		dockerCli:      dockerCli,
		clock:          clockwork.NewRealClock(),
		maxConcurrency: -1,
		dryRun:         false,
	}
}

type composeService struct {
	dockerCli   command.Cli
	desktopCli  *desktop.Client
	experiments *experimental.State

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
		errs = append(errs, s.dockerCli.Client().Close())
	}
	if s.isDesktopIntegrationActive() {
		errs = append(errs, s.desktopCli.Close())
	}
	return errors.Join(errs...)
}

func (s *composeService) apiClient() client.APIClient {
	return s.dockerCli.Client()
}

func (s *composeService) configFile() *configfile.ConfigFile {
	return s.dockerCli.ConfigFile()
}

func (s *composeService) MaxConcurrency(i int) {
	s.maxConcurrency = i
}

func (s *composeService) DryRunMode(ctx context.Context, dryRun bool) (context.Context, error) {
	s.dryRun = dryRun
	if dryRun {
		cli, err := command.NewDockerCli()
		if err != nil {
			return ctx, err
		}

		options := flags.NewClientOptions()
		options.Context = s.dockerCli.CurrentContext()
		err = cli.Initialize(options, command.WithInitializeClient(func(cli *command.DockerCli) (client.APIClient, error) {
			return api.NewDryRunClient(s.apiClient(), s.dockerCli)
		}))
		if err != nil {
			return ctx, err
		}
		s.dockerCli = cli
	}
	return context.WithValue(ctx, api.DryRunKey{}, dryRun), nil
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
		return s.dockerCli.Out()
	}
	return s.dockerCli.Err()
}

func getCanonicalContainerName(c moby.Container) string {
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

func getContainerNameWithoutProject(c moby.Container) string {
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
		serviceLabel := c.Labels[api.ServiceLabel]
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
		if len(dependencies) > 0 {
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
		version, err := s.dockerCli.Client().ServerVersion(ctx)
		if err != nil {
			runtimeVersion.err = err
		}
		runtimeVersion.val = version.APIVersion
	})
	return runtimeVersion.val, runtimeVersion.err

}

func (s *composeService) isDesktopIntegrationActive() bool {
	return s.desktopCli != nil
}

func (s *composeService) isDesktopUIEnabled() bool {
	if !s.isDesktopIntegrationActive() {
		return false
	}
	return s.experiments.ComposeUI()
}
