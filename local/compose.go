// +build local

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

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/formatter"
	"github.com/docker/compose-cli/progress"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"github.com/sanathkr/go-yaml"
)

func (s *local) Up(ctx context.Context, project *types.Project, detach bool) error {
	for k, network := range project.Networks {
		if !network.External.External && network.Name != "" {
			network.Name = fmt.Sprintf("%s_%s", project.Name, k)
			project.Networks[k] = network
		}
		err := s.ensureNetwork(ctx, network)
		if err != nil {
			return err
		}
	}

	for k, volume := range project.Volumes {
		if !volume.External.External && volume.Name != "" {
			volume.Name = fmt.Sprintf("%s_%s", project.Name, k)
			project.Volumes[k] = volume
		}
		err := s.ensureVolume(ctx, volume)
		if err != nil {
			return err
		}
	}

	for _, service := range project.Services {
		err := s.applyPullPolicy(ctx, service)
		if err != nil {
			return err
		}
	}

	err := inDependencyOrder(ctx, project, func(service types.ServiceConfig) error {
		return s.ensureService(ctx, project, service)
	})
	return err
}

func getContainerName(c moby.Container) string {
	// Names return container canonical name /foo  + link aliases /linked_by/foo
	for _, name := range c.Names {
		if strings.LastIndex(name, "/") == 0 {
			return name[1:]
		}
	}
	return c.Names[0][1:]
}

func (s *local) applyPullPolicy(ctx context.Context, service types.ServiceConfig) error {
	w := progress.ContextWriter(ctx)
	// TODO build vs pull should be controlled by pull policy
	// if service.Build {}
	if service.Image != "" {
		_, _, err := s.containerService.apiClient.ImageInspectWithRaw(ctx, service.Image)
		if err != nil {
			if errdefs.IsNotFound(err) {
				stream, err := s.containerService.apiClient.ImagePull(ctx, service.Image, moby.ImagePullOptions{})
				if err != nil {
					return err
				}
				dec := json.NewDecoder(stream)
				for {
					var jm jsonmessage.JSONMessage
					if err := dec.Decode(&jm); err != nil {
						if err == io.EOF {
							break
						}
						return err
					}
					toProgressEvent(jm, w)
				}
			}
		}
	}
	return nil
}

func toProgressEvent(jm jsonmessage.JSONMessage, w progress.Writer) {
	if jm.Progress != nil {
		if jm.Progress.Total != 0 {
			percentage := int(float64(jm.Progress.Current)/float64(jm.Progress.Total)*100) / 2
			numSpaces := 50 - percentage
			w.Event(progress.Event{
				ID:         jm.ID,
				Text:       jm.Status,
				Status:     0,
				StatusText: fmt.Sprintf("[%s>%s] ", strings.Repeat("=", percentage), strings.Repeat(" ", numSpaces)),
				Done:       jm.Status == "Pull complete",
			})
		} else {
			if jm.Error != nil {
				w.Event(progress.Event{
					ID:         jm.ID,
					Text:       jm.Status,
					Status:     progress.Error,
					StatusText: jm.Error.Message,
					Done:       true,
				})
			} else if jm.Status == "Pull complete" || jm.Status == "Already exists" {
				w.Event(progress.Event{
					ID:     jm.ID,
					Text:   jm.Status,
					Status: progress.Done,
					Done:   true,
				})
			} else {
				w.Event(progress.Event{
					ID:     jm.ID,
					Text:   jm.Status,
					Status: progress.Working,
					Done:   false,
				})
			}
		}
	}
}

func (s *local) Down(ctx context.Context, projectName string) error {
	list, err := s.containerService.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return err
	}
	for _, c := range list {
		err := s.containerService.Stop(ctx, c.ID, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *local) Logs(ctx context.Context, projectName string, w io.Writer) error {
	list, err := s.containerService.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	consumer := formatter.NewLogConsumer(w)
	for _, c := range list {
		service := c.Labels["com.docker.compose.service"]
		containerId := c.ID
		go func() {
			s.containerService.Logs(ctx,containerId, containers.LogsRequest{
				Follow: true,
				Writer: consumer.GetWriter(service, containerId),
			})
			wg.Done()
		}()
		wg.Add(1)
	}
	wg.Wait()
	return nil
}

func (s *local) Ps(ctx context.Context, projectName string) ([]compose.ServiceStatus, error) {
	list, err := s.containerService.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return nil, err
	}
	var status []compose.ServiceStatus
	for _, c := range list {
		// TODO group by service
		status = append(status, compose.ServiceStatus{
			ID:         c.ID,
			Name:       c.Labels["com.docker.compose.service"],
			Replicas:   0,
			Desired:    0,
			Ports:      nil,
			Publishers: nil,
		})
	}
	return status, nil
}

func (s *local) List(ctx context.Context, projectName string) ([]compose.Stack, error) {
	_, err := s.containerService.apiClient.ContainerList(ctx, moby.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	var stacks []compose.Stack
	// TODO rebuild stacks based on containers
	return stacks, nil
}

func (s *local) Convert(ctx context.Context, project *types.Project, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(project, "", "  ")
	case "yaml":
		return yaml.Marshal(project)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func getContainerCreateOptions(p *types.Project, s types.ServiceConfig, inherit *moby.Container) (*container.Config, *container.HostConfig, *network.NetworkingConfig, error) {
	hash, err := jsonHash(s)
	if err != nil {
		return nil, nil, nil, err
	}
	labels := map[string]string{
		"com.docker.compose.project":     p.Name,
		"com.docker.compose.service":     s.Name,
		"com.docker.compose.config-hash": hash,
	}

	var (
		runCmd     strslice.StrSlice
		entrypoint strslice.StrSlice
	)
	if len(s.Command) > 0 {
		runCmd = strslice.StrSlice(s.Command)
	}
	if len(s.Entrypoint) > 0 {
		entrypoint = strslice.StrSlice(s.Entrypoint)
	}
	image := s.Image
	if s.Image == "" {
		image = fmt.Sprintf("%s_%s", p.Name, s.Name)
	}

	var (
		tty         = s.Tty
		stdinOpen   = s.StdinOpen
		attachStdin = false
	)

	containerConfig := container.Config{
		Hostname:        s.Hostname,
		Domainname:      s.DomainName,
		User:            s.User,
		ExposedPorts:    buildContainerPorts(s),
		Tty:             tty,
		OpenStdin:       stdinOpen,
		StdinOnce:       true,
		AttachStdin:     attachStdin,
		AttachStderr:    true,
		AttachStdout:    true,
		Cmd:             runCmd,
		Image:           image,
		WorkingDir:      s.WorkingDir,
		Entrypoint:      entrypoint,
		NetworkDisabled: s.NetworkMode == "disabled",
		MacAddress:      s.MacAddress,
		Labels:          labels,
		StopSignal:      s.StopSignal,
		//		Env:             s.Environment, FIXME conversion
		//		Healthcheck:     s.HealthCheck, FIXME conversion
		// Volumes:         // FIXME unclear to me the overlap with HostConfig.Mounts
		// StopTimeout: 	 s.StopGracePeriod FIXME conversion
	}

	mountOptions, err := buildContainerMountOptions(p, s, inherit)
	if err != nil {
		return nil, nil, nil, err
	}

	bindings, err := buildContainerBindingOptions(s)
	if err != nil {
		return nil, nil, nil, err
	}

	networkMode := getNetworkMode(p, s)
	hostConfig := container.HostConfig{
		Mounts:         mountOptions,
		CapAdd:         strslice.StrSlice(s.CapAdd),
		CapDrop:        strslice.StrSlice(s.CapDrop),
		NetworkMode:    networkMode,
		Init:           s.Init,
		ReadonlyRootfs: s.ReadOnly,
		// ShmSize: , TODO
		Sysctls:      s.Sysctls,
		PortBindings: bindings,
	}

	networkConfig := buildDefaultNetworkConfig(s, networkMode)
	return &containerConfig, &hostConfig, networkConfig, nil
}

func buildContainerPorts(s types.ServiceConfig) nat.PortSet {
	ports := nat.PortSet{}
	for _, p := range s.Ports {
		p := nat.Port(fmt.Sprintf("%d/%s", p.Target, p.Protocol))
		ports[p] = struct{}{}
	}
	return ports
}

func buildContainerBindingOptions(s types.ServiceConfig) (nat.PortMap, error) {
	bindings := nat.PortMap{}
	for _, port := range s.Ports {
		p := nat.Port(fmt.Sprintf("%d/%s", port.Target, port.Protocol))
		bind := []nat.PortBinding{}
		binding := nat.PortBinding{}
		if port.Published > 0 {
			binding.HostPort = fmt.Sprint(port.Published)
		}
		bind = append(bind, binding)
		bindings[p] = bind
	}
	return bindings, nil
}

func buildContainerMountOptions(p *types.Project, s types.ServiceConfig, inherit *moby.Container) ([]mount.Mount, error) {
	mounts := []mount.Mount{}
	var inherited []string
	if inherit != nil {
		for _, m := range inherit.Mounts {
			if m.Type == "tmpfs" {
				continue
			}
			src := m.Source
			if m.Type == "volume" {
				src = m.Name
			}
			mounts = append(mounts, mount.Mount{
				Type:     m.Type,
				Source:   src,
				Target:   m.Destination,
				ReadOnly: !m.RW,
			})
			inherited = append(inherited, m.Destination)
		}
	}

	for _, v := range s.Volumes {
		if contains(inherited, v.Target) {
			continue
		}
		source := v.Source
		if v.Type == "bind" && !filepath.IsAbs(source) {
			// FIXME handle ~/
			source = filepath.Join(p.WorkingDir, source)
		}

		mounts = append(mounts, mount.Mount{
			Type:          mount.Type(v.Type),
			Source:        source,
			Target:        v.Target,
			ReadOnly:      v.ReadOnly,
			Consistency:   mount.Consistency(v.Consistency),
			BindOptions:   buildBindOption(v.Bind),
			VolumeOptions: buildVolumeOptions(v.Volume),
			TmpfsOptions:  buildTmpfsOptions(v.Tmpfs),
		})
	}
	return mounts, nil
}

func buildBindOption(bind *types.ServiceVolumeBind) *mount.BindOptions {
	if bind == nil {
		return nil
	}
	return &mount.BindOptions{
		Propagation: mount.Propagation(bind.Propagation),
		// NonRecursive: false, FIXME missing from model ?
	}
}

func buildVolumeOptions(vol *types.ServiceVolumeVolume) *mount.VolumeOptions {
	if vol == nil {
		return nil
	}
	return &mount.VolumeOptions{
		NoCopy: vol.NoCopy,
		// Labels:       , // FIXME missing from model ?
		// DriverConfig: , // FIXME missing from model ?
	}
}

func buildTmpfsOptions(tmpfs *types.ServiceVolumeTmpfs) *mount.TmpfsOptions {
	if tmpfs == nil {
		return nil
	}
	return &mount.TmpfsOptions{
		SizeBytes: tmpfs.Size,
		// Mode:      , // FIXME missing from model ?
	}
}

func buildDefaultNetworkConfig(s types.ServiceConfig, networkMode container.NetworkMode) *network.NetworkingConfig {
	config := map[string]*network.EndpointSettings{}
	net := string(networkMode)
	config[net] = &network.EndpointSettings{
		Aliases: getAliases(s, s.Networks[net]),
	}

	return &network.NetworkingConfig{
		EndpointsConfig: config,
	}
}

func getAliases(s types.ServiceConfig, c *types.ServiceNetworkConfig) []string {
	aliases := []string{s.Name}
	if c != nil {
		aliases = append(aliases, c.Aliases...)
	}
	return aliases
}

func getNetworkMode(p *types.Project, service types.ServiceConfig) container.NetworkMode {
	mode := service.NetworkMode
	if mode == "" {
		if len(p.Networks) > 0 {
			for name := range getNetworksForService(service) {
				return container.NetworkMode(p.Networks[name].Name)
			}
		}
		return container.NetworkMode("none")
	}

	/// FIXME incomplete implementation
	if strings.HasPrefix(mode, "service:") {
		panic("Not yet implemented")
	}
	if strings.HasPrefix(mode, "container:") {
		panic("Not yet implemented")
	}

	return container.NetworkMode(mode)
}

func getNetworksForService(s types.ServiceConfig) map[string]*types.ServiceNetworkConfig {
	if len(s.Networks) > 0 {
		return s.Networks
	}
	return map[string]*types.ServiceNetworkConfig{"default": nil}
}

func (s *local) ensureNetwork(ctx context.Context, n types.NetworkConfig) error {
	_, err := s.containerService.apiClient.NetworkInspect(ctx, n.Name, moby.NetworkInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			createOpts := moby.NetworkCreate{
				// TODO NameSpace Labels
				Labels:     n.Labels,
				Driver:     n.Driver,
				Options:    n.DriverOpts,
				Internal:   n.Internal,
				Attachable: n.Attachable,
			}

			if n.Ipam.Driver != "" || len(n.Ipam.Config) > 0 {
				createOpts.IPAM = &network.IPAM{}
			}

			if n.Ipam.Driver != "" {
				createOpts.IPAM.Driver = n.Ipam.Driver
			}

			for _, ipamConfig := range n.Ipam.Config {
				config := network.IPAMConfig{
					Subnet: ipamConfig.Subnet,
				}
				createOpts.IPAM.Config = append(createOpts.IPAM.Config, config)
			}
			w := progress.ContextWriter(ctx)
			w.Event(progress.Event{
				ID:         fmt.Sprintf("Network %q", n.Name),
				Status:     progress.Working,
				StatusText: "Create",
				Done:       false,
			})
			if _, err := s.containerService.apiClient.NetworkCreate(context.Background(), n.Name, createOpts); err != nil {
				return errors.Wrapf(err, "failed to create network %s", n.Name)
			}
			w.Event(progress.Event{
				ID:         fmt.Sprintf("Network %q", n.Name),
				Status:     progress.Working,
				StatusText: "Created",
				Done:       true,
			})
			return nil
		} else {
			return err
		}
	}
	return nil
}

func (s *local) ensureVolume(ctx context.Context, volume types.VolumeConfig) error {
	// TODO could identify volume by label vs name
	_, err := s.volumeService.Inspect(ctx, volume.Name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			w := progress.ContextWriter(ctx)
			w.Event(progress.Event{
				ID:         fmt.Sprintf("Volume %q", volume.Name),
				Status:     progress.Working,
				StatusText: "Create",
				Done:       false,
			})
			// TODO we miss support for driver_opts and labels
			_, err := s.volumeService.Create(ctx, volume.Name, nil)
			w.Event(progress.Event{
				ID:         fmt.Sprintf("Volume %q", volume.Name),
				Status:     progress.Done,
				StatusText: "Created",
				Done:       true,
			})
			if err != nil {
				return err
			}
		}
		return err
	}
	return nil
}
