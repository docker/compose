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
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/api/compose"
	convert "github.com/docker/compose-cli/local/moby"
	apitypes "github.com/docker/docker/api/types"
	moby "github.com/docker/docker/pkg/stringid"
)

func (s *composeService) CreateOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) (string, error) {
	name := opts.Name
	service, err := project.GetService(name)
	if err != nil {
		return "", err
	}

	err = s.ensureRequiredNetworks(ctx, project, service)
	if err != nil {
		return "", err
	}
	err = s.ensureRequiredVolumes(ctx, project, service)
	if err != nil {
		return "", err
	}
	// ensure required services are up and running before creating the oneoff container
	err = s.ensureRequiredServices(ctx, project, service)
	if err != nil {
		return "", err
	}

	//apply options to service config
	updateOneOffServiceConfig(&service, project.Name, opts)

	err = s.createContainer(ctx, project, service, service.ContainerName, 1)
	if err != nil {
		return "", err
	}

	return service.ContainerName, err
}

func (s *composeService) Run(ctx context.Context, container string, detach bool) error {
	if detach {
		// start container
		return s.apiClient.ContainerStart(ctx, container, apitypes.ContainerStartOptions{})
	}

	cnx, err := s.apiClient.ContainerAttach(ctx, container, apitypes.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		return err
	}
	defer cnx.Close()

	stdout := convert.ContainerStdout{HijackedResponse: cnx}
	stdin := convert.ContainerStdin{HijackedResponse: cnx}

	readChannel := make(chan error, 10)
	writeChannel := make(chan error, 10)

	go func() {
		_, err := io.Copy(os.Stdout, cnx.Reader)
		readChannel <- err
	}()

	go func() {
		_, err := io.Copy(stdin, os.Stdin)
		writeChannel <- err
	}()

	go func() {
		<-ctx.Done()
		stdout.Close() //nolint:errcheck
		stdin.Close()  //nolint:errcheck
	}()

	// start container
	err = s.apiClient.ContainerStart(ctx, container, apitypes.ContainerStartOptions{})
	if err != nil {
		return err
	}

	for {
		select {
		case err := <-readChannel:
			return err
		case err := <-writeChannel:
			return err
		}
	}
}

func updateOneOffServiceConfig(service *types.ServiceConfig, projectName string, opts compose.RunOptions) {
	if len(opts.Command) > 0 {
		// custom command to run
		service.Command = opts.Command
	}
	//service.Environment = opts.Environment
	slug := moby.GenerateRandomID()
	service.Scale = 1
	service.ContainerName = fmt.Sprintf("%s_%s_run_%s", projectName, service.Name, moby.TruncateID(slug))
	service.Labels = types.Labels{
		"com.docker.compose.slug":   slug,
		"com.docker.compose.oneoff": "True",
	}
	service.Tty = true
	service.StdinOpen = true
}

func (s *composeService) ensureRequiredServices(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	requiredServices := getDependencyNames(project, service, func() []string {
		return service.GetDependencies()
	})
	if len(requiredServices) > 0 {
		// dependencies here
		services, err := project.GetServices(requiredServices)
		if err != nil {
			return err
		}
		project.Services = services
		err = s.ensureImagesExists(ctx, project)
		if err != nil {
			return err
		}

		err = InDependencyOrder(ctx, project, func(c context.Context, svc types.ServiceConfig) error {
			return s.ensureService(c, project, svc)
		})
		if err != nil {
			return err
		}
		return s.Start(ctx, project, nil)
	}
	return nil
}

func (s *composeService) ensureRequiredNetworks(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	networks := getDependentNetworkNames(project, service)
	for k, network := range project.Networks {
		if !contains(networks, network.Name) {
			continue
		}
		if !network.External.External && network.Name != "" {
			network.Name = fmt.Sprintf("%s_%s", project.Name, k)
			project.Networks[k] = network
		}
		network.Labels = network.Labels.Add(networkLabel, k)
		network.Labels = network.Labels.Add(projectLabel, project.Name)
		network.Labels = network.Labels.Add(versionLabel, ComposeVersion)

		err := s.ensureNetwork(ctx, network)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) ensureRequiredVolumes(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	volumes := getDependentVolumeNames(project, service)

	for k, volume := range project.Volumes {
		if !contains(volumes, volume.Name) {
			continue
		}
		if !volume.External.External && volume.Name != "" {
			volume.Name = fmt.Sprintf("%s_%s", project.Name, k)
			project.Volumes[k] = volume
		}
		volume.Labels = volume.Labels.Add(volumeLabel, k)
		volume.Labels = volume.Labels.Add(projectLabel, project.Name)
		volume.Labels = volume.Labels.Add(versionLabel, ComposeVersion)
		err := s.ensureVolume(ctx, volume)
		if err != nil {
			return err
		}
	}
	return nil
}

type filterDependency func() []string

func getDependencyNames(project *types.Project, service types.ServiceConfig, f filterDependency) []string {
	names := f()
	serviceNames := service.GetDependencies()
	if len(serviceNames) == 0 {
		return names
	}
	if len(serviceNames) > 0 {
		services, _ := project.GetServices(serviceNames)
		for _, s := range services {
			svc := getDependencyNames(project, s, f)
			names = append(names, svc...)
		}
	}
	sort.Strings(names)
	return unique(names)
}

func getDependentNetworkNames(project *types.Project, service types.ServiceConfig) []string {
	return getDependencyNames(project, service, func() []string {
		names := []string{}
		for n := range service.Networks {
			if contains(project.NetworkNames(), n) {
				names = append(names, n)
			}
		}
		return names
	})
}

func getDependentVolumeNames(project *types.Project, service types.ServiceConfig) []string {
	return getDependencyNames(project, service, func() []string {
		names := []string{}
		for _, v := range service.Volumes {
			if contains(project.VolumeNames(), v.Source) {
				names = append(names, v.Source)
			}
		}
		return names
	})
}
