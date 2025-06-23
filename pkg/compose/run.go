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
	"os/signal"
	"slices"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	cmd "github.com/docker/cli/cli/command/container"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/pkg/stringid"
)

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	containerID, err := s.prepareRun(ctx, project, opts)
	if err != nil {
		return 0, err
	}

	// remove cancellable context signal handler so we can forward signals to container without compose to exit
	signal.Reset()

	sigc := make(chan os.Signal, 128)
	signal.Notify(sigc)
	go cmd.ForwardAllSignals(ctx, s.apiClient(), containerID, sigc)
	defer signal.Stop(sigc)

	err = cmd.RunStart(ctx, s.dockerCli, &cmd.StartOptions{
		OpenStdin:  !opts.Detach && opts.Interactive,
		Attach:     !opts.Detach,
		Containers: []string{containerID},
	})
	var stErr cli.StatusError
	if errors.As(err, &stErr) {
		return stErr.StatusCode, nil
	}
	return 0, err
}

func (s *composeService) prepareRun(ctx context.Context, project *types.Project, opts api.RunOptions) (string, error) {
	// Temporary implementation of use_api_socket until we get actual support inside docker engine
	project, err := s.useAPISocket(project)
	if err != nil {
		return "", err
	}

	err = progress.Run(ctx, func(ctx context.Context) error {
		return s.startDependencies(ctx, project, opts)
	}, s.stdinfo())
	if err != nil {
		return "", err
	}

	service, err := project.GetService(opts.Service)
	if err != nil {
		return "", err
	}

	applyRunOptions(project, &service, opts)

	if err := s.stdin().CheckTty(opts.Interactive, service.Tty); err != nil {
		return "", err
	}

	slug := stringid.GenerateRandomID()
	if service.ContainerName == "" {
		service.ContainerName = fmt.Sprintf("%[1]s%[4]s%[2]s%[4]srun%[4]s%[3]s", project.Name, service.Name, stringid.TruncateID(slug), api.Separator)
	}
	one := 1
	service.Scale = &one
	service.Restart = ""
	if service.Deploy != nil {
		service.Deploy.RestartPolicy = nil
	}
	service.CustomLabels = service.CustomLabels.
		Add(api.SlugLabel, slug).
		Add(api.OneoffLabel, "True")

	if err := s.ensureImagesExists(ctx, project, opts.Build, opts.QuietPull); err != nil { // all dependencies already checked, but might miss service img
		return "", err
	}

	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return "", err
	}

	if !opts.NoDeps {
		if err := s.waitDependencies(ctx, project, service.Name, service.DependsOn, observedState, 0); err != nil {
			return "", err
		}
	}
	createOpts := createOptions{
		AutoRemove:        opts.AutoRemove,
		AttachStdin:       opts.Interactive,
		UseNetworkAliases: opts.UseNetworkAliases,
		Labels:            mergeLabels(service.Labels, service.CustomLabels),
	}

	err = newConvergence(project.ServiceNames(), observedState, nil, nil, s).resolveServiceReferences(&service)
	if err != nil {
		return "", err
	}

	created, err := s.createContainer(ctx, project, service, service.ContainerName, -1, createOpts)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func applyRunOptions(project *types.Project, service *types.ServiceConfig, opts api.RunOptions) {
	service.Tty = opts.Tty
	service.StdinOpen = opts.Interactive
	service.ContainerName = opts.Name

	if len(opts.Command) > 0 {
		service.Command = opts.Command
	}
	if opts.User != "" {
		service.User = opts.User
	}

	if len(opts.CapAdd) > 0 {
		service.CapAdd = append(service.CapAdd, opts.CapAdd...)
		service.CapDrop = slices.DeleteFunc(service.CapDrop, func(e string) bool { return slices.Contains(opts.CapAdd, e) })
	}
	if len(opts.CapDrop) > 0 {
		service.CapDrop = append(service.CapDrop, opts.CapDrop...)
		service.CapAdd = slices.DeleteFunc(service.CapAdd, func(e string) bool { return slices.Contains(opts.CapDrop, e) })
	}
	if opts.WorkingDir != "" {
		service.WorkingDir = opts.WorkingDir
	}
	if opts.Entrypoint != nil {
		service.Entrypoint = opts.Entrypoint
		if len(opts.Command) == 0 {
			service.Command = []string{}
		}
	}
	if len(opts.Environment) > 0 {
		cmdEnv := types.NewMappingWithEquals(opts.Environment)
		serviceOverrideEnv := cmdEnv.Resolve(func(s string) (string, bool) {
			v, ok := envResolver(project.Environment)(s)
			return v, ok
		}).RemoveEmpty()
		if service.Environment == nil {
			service.Environment = types.MappingWithEquals{}
		}
		service.Environment.OverrideBy(serviceOverrideEnv)
	}
	for k, v := range opts.Labels {
		service.Labels = service.Labels.Add(k, v)
	}
}

func (s *composeService) startDependencies(ctx context.Context, project *types.Project, options api.RunOptions) error {
	dependencies := types.Services{}
	var requestedService types.ServiceConfig
	for name, service := range project.Services {
		if name != options.Service {
			dependencies[name] = service
		} else {
			requestedService = service
		}
	}

	if len(dependencies) > 0 {
		project.Services = dependencies
		project.DisabledServices[options.Service] = requestedService
	}
	project.DisabledServices[options.Service] = requestedService
	delete(project.Services, options.Service)

	err = s.Create(ctx, project, api.CreateOptions{
		Build:         options.Build,
		IgnoreOrphans: options.IgnoreOrphans,
		RemoveOrphans: options.RemoveOrphans,
		QuietPull:     options.QuietPull,
	})
	if err != nil {
		return err
	}

	if len(dependencies) > 0 {
		return s.Start(ctx, project.Name, api.StartOptions{
			Project: project,
		})
	}
	return nil
}
