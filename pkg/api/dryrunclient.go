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

package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"

	"github.com/docker/buildx/builder"
	"github.com/docker/buildx/util/imagetools"
	"github.com/docker/cli/cli/command"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/checkpoint"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	DRYRUN_PREFIX = " DRY-RUN MODE - "
)

var _ client.APIClient = &DryRunClient{}

type DryRunKey struct{}

// DryRunClient implements APIClient by delegating to implementation functions. This allows lazy init and per-method overrides
type DryRunClient struct {
	apiClient  client.APIClient
	containers []moby.Container
	execs      sync.Map
	resolver   *imagetools.Resolver
}

type execDetails struct {
	container string
	command   []string
}

// NewDryRunClient produces a DryRunClient
func NewDryRunClient(apiClient client.APIClient, cli command.Cli) (*DryRunClient, error) {
	b, err := builder.New(cli, builder.WithSkippedValidation())
	if err != nil {
		return nil, err
	}
	configFile, err := b.ImageOpt()
	if err != nil {
		return nil, err
	}
	return &DryRunClient{
		apiClient:  apiClient,
		containers: []moby.Container{},
		execs:      sync.Map{},
		resolver:   imagetools.New(configFile),
	}, nil
}

func getCallingFunction() string {
	pc, _, _, _ := runtime.Caller(2)
	fullName := runtime.FuncForPC(pc).Name()
	return fullName[strings.LastIndex(fullName, ".")+1:]
}

// All methods and functions which need to be overridden for dry run.

func (d *DryRunClient) ContainerAttach(ctx context.Context, container string, options containerType.AttachOptions) (moby.HijackedResponse, error) {
	return moby.HijackedResponse{}, errors.New("interactive run is not supported in dry-run mode")
}

func (d *DryRunClient) ContainerCreate(ctx context.Context, config *containerType.Config, hostConfig *containerType.HostConfig,
	networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (containerType.CreateResponse, error) {
	d.containers = append(d.containers, moby.Container{
		ID:     containerName,
		Names:  []string{containerName},
		Labels: config.Labels,
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{},
	})
	return containerType.CreateResponse{ID: containerName}, nil
}

func (d *DryRunClient) ContainerInspect(ctx context.Context, container string) (moby.ContainerJSON, error) {
	containerJSON, err := d.apiClient.ContainerInspect(ctx, container)
	if err != nil {
		id := "dryRunId"
		for _, c := range d.containers {
			if c.ID == container {
				id = container
			}
		}
		return moby.ContainerJSON{
			ContainerJSONBase: &moby.ContainerJSONBase{
				ID:   id,
				Name: container,
				State: &moby.ContainerState{
					Status: "running", // needed for --wait option
					Health: &moby.Health{
						Status: moby.Healthy, // needed for healthcheck control
					},
				},
			},
			Mounts:          nil,
			Config:          &containerType.Config{},
			NetworkSettings: &moby.NetworkSettings{},
		}, nil
	}
	return containerJSON, err
}

func (d *DryRunClient) ContainerKill(ctx context.Context, container, signal string) error {
	return nil
}

func (d *DryRunClient) ContainerList(ctx context.Context, options containerType.ListOptions) ([]moby.Container, error) {
	caller := getCallingFunction()
	switch caller {
	case "start":
		return d.containers, nil
	case "getContainers":
		if len(d.containers) == 0 {
			var err error
			d.containers, err = d.apiClient.ContainerList(ctx, options)
			return d.containers, err
		}
	}
	return d.apiClient.ContainerList(ctx, options)
}

func (d *DryRunClient) ContainerPause(ctx context.Context, container string) error {
	return nil
}

func (d *DryRunClient) ContainerRemove(ctx context.Context, container string, options containerType.RemoveOptions) error {
	return nil
}

func (d *DryRunClient) ContainerRename(ctx context.Context, container, newContainerName string) error {
	return nil
}

func (d *DryRunClient) ContainerRestart(ctx context.Context, container string, options containerType.StopOptions) error {
	return nil
}

func (d *DryRunClient) ContainerStart(ctx context.Context, container string, options containerType.StartOptions) error {
	return nil
}

func (d *DryRunClient) ContainerStop(ctx context.Context, container string, options containerType.StopOptions) error {
	return nil
}

func (d *DryRunClient) ContainerUnpause(ctx context.Context, container string) error {
	return nil
}

func (d *DryRunClient) CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, containerType.PathStat, error) {
	rc := io.NopCloser(strings.NewReader(""))
	if _, err := d.ContainerStatPath(ctx, container, srcPath); err != nil {
		return rc, containerType.PathStat{}, fmt.Errorf(" %s Could not find the file %s in container %s", DRYRUN_PREFIX, srcPath, container)
	}
	return rc, containerType.PathStat{}, nil
}

func (d *DryRunClient) CopyToContainer(ctx context.Context, container, path string, content io.Reader, options containerType.CopyToContainerOptions) error {
	if _, err := d.ContainerStatPath(ctx, container, path); err != nil {
		return fmt.Errorf(" %s Could not find the file %s in container %s", DRYRUN_PREFIX, path, container)
	}
	return nil
}

func (d *DryRunClient) ImageBuild(ctx context.Context, reader io.Reader, options moby.ImageBuildOptions) (moby.ImageBuildResponse, error) {
	jsonMessage, err := json.Marshal(&jsonmessage.JSONMessage{
		Status:   fmt.Sprintf("%[1]sSuccessfully built: dryRunID\n%[1]sSuccessfully tagged: %[2]s\n", DRYRUN_PREFIX, options.Tags[0]),
		Progress: &jsonmessage.JSONProgress{},
		ID:       "",
	})
	if err != nil {
		return moby.ImageBuildResponse{}, err
	}
	rc := io.NopCloser(bytes.NewReader(jsonMessage))

	return moby.ImageBuildResponse{
		Body:   rc,
		OSType: "",
	}, nil
}

func (d *DryRunClient) ImageInspectWithRaw(ctx context.Context, imageName string) (moby.ImageInspect, []byte, error) {
	caller := getCallingFunction()
	switch caller {
	case "pullServiceImage", "buildContainerVolumes":
		return moby.ImageInspect{ID: "dryRunId"}, nil, nil
	default:
		return d.apiClient.ImageInspectWithRaw(ctx, imageName)
	}

}

func (d *DryRunClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	if _, _, err := d.resolver.Resolve(ctx, ref); err != nil {
		return nil, err
	}
	rc := io.NopCloser(strings.NewReader(""))
	return rc, nil
}

func (d *DryRunClient) ImagePush(ctx context.Context, ref string, options image.PushOptions) (io.ReadCloser, error) {
	if _, _, err := d.resolver.Resolve(ctx, ref); err != nil {
		return nil, err
	}
	jsonMessage, err := json.Marshal(&jsonmessage.JSONMessage{
		Status: "Pushed",
		Progress: &jsonmessage.JSONProgress{
			Current:    100,
			Total:      100,
			Start:      0,
			HideCounts: false,
			Units:      "Mb",
		},
		ID: ref,
	})
	if err != nil {
		return nil, err
	}
	rc := io.NopCloser(bytes.NewReader(jsonMessage))
	return rc, nil
}

func (d *DryRunClient) ImageRemove(ctx context.Context, imageName string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	return nil, nil
}

func (d *DryRunClient) NetworkConnect(ctx context.Context, networkName, container string, config *network.EndpointSettings) error {
	return nil
}

func (d *DryRunClient) NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error) {
	return network.CreateResponse{
		ID:      name,
		Warning: "",
	}, nil
}

func (d *DryRunClient) NetworkDisconnect(ctx context.Context, networkName, container string, force bool) error {
	return nil
}

func (d *DryRunClient) NetworkRemove(ctx context.Context, networkName string) error {
	return nil
}

func (d *DryRunClient) VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
	return volume.Volume{
		ClusterVolume: nil,
		Driver:        options.Driver,
		Labels:        options.Labels,
		Name:          options.Name,
		Options:       options.DriverOpts,
	}, nil
}

func (d *DryRunClient) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	return nil
}

func (d *DryRunClient) ContainerExecCreate(ctx context.Context, container string, config containerType.ExecOptions) (moby.IDResponse, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	id := fmt.Sprintf("%x", b)
	d.execs.Store(id, execDetails{
		container: container,
		command:   config.Cmd,
	})
	return moby.IDResponse{
		ID: id,
	}, nil
}

func (d *DryRunClient) ContainerExecStart(ctx context.Context, execID string, config containerType.ExecStartOptions) error {
	v, ok := d.execs.LoadAndDelete(execID)
	if !ok {
		return fmt.Errorf("invalid exec ID %q", execID)
	}
	details := v.(execDetails)
	fmt.Printf("%sExecuting command %q in %s (detached mode)\n", DRYRUN_PREFIX, details.command, details.container)
	return nil
}

// Functions delegated to original APIClient (not used by Compose or not modifying the Compose stack

func (d *DryRunClient) ConfigList(ctx context.Context, options moby.ConfigListOptions) ([]swarm.Config, error) {
	return d.apiClient.ConfigList(ctx, options)
}

func (d *DryRunClient) ConfigCreate(ctx context.Context, config swarm.ConfigSpec) (moby.ConfigCreateResponse, error) {
	return d.apiClient.ConfigCreate(ctx, config)
}

func (d *DryRunClient) ConfigRemove(ctx context.Context, id string) error {
	return d.apiClient.ConfigRemove(ctx, id)
}

func (d *DryRunClient) ConfigInspectWithRaw(ctx context.Context, name string) (swarm.Config, []byte, error) {
	return d.apiClient.ConfigInspectWithRaw(ctx, name)
}

func (d *DryRunClient) ConfigUpdate(ctx context.Context, id string, version swarm.Version, config swarm.ConfigSpec) error {
	return d.apiClient.ConfigUpdate(ctx, id, version, config)
}

func (d *DryRunClient) ContainerCommit(ctx context.Context, container string, options containerType.CommitOptions) (moby.IDResponse, error) {
	return d.apiClient.ContainerCommit(ctx, container, options)
}

func (d *DryRunClient) ContainerDiff(ctx context.Context, container string) ([]containerType.FilesystemChange, error) {
	return d.apiClient.ContainerDiff(ctx, container)
}

func (d *DryRunClient) ContainerExecAttach(ctx context.Context, execID string, config containerType.ExecStartOptions) (moby.HijackedResponse, error) {
	return moby.HijackedResponse{}, errors.New("interactive exec is not supported in dry-run mode")
}

func (d *DryRunClient) ContainerExecInspect(ctx context.Context, execID string) (containerType.ExecInspect, error) {
	return d.apiClient.ContainerExecInspect(ctx, execID)
}

func (d *DryRunClient) ContainerExecResize(ctx context.Context, execID string, options containerType.ResizeOptions) error {
	return d.apiClient.ContainerExecResize(ctx, execID, options)
}

func (d *DryRunClient) ContainerExport(ctx context.Context, container string) (io.ReadCloser, error) {
	return d.apiClient.ContainerExport(ctx, container)
}

func (d *DryRunClient) ContainerInspectWithRaw(ctx context.Context, container string, getSize bool) (moby.ContainerJSON, []byte, error) {
	return d.apiClient.ContainerInspectWithRaw(ctx, container, getSize)
}

func (d *DryRunClient) ContainerLogs(ctx context.Context, container string, options containerType.LogsOptions) (io.ReadCloser, error) {
	return d.apiClient.ContainerLogs(ctx, container, options)
}

func (d *DryRunClient) ContainerResize(ctx context.Context, container string, options containerType.ResizeOptions) error {
	return d.apiClient.ContainerResize(ctx, container, options)
}

func (d *DryRunClient) ContainerStatPath(ctx context.Context, container, path string) (containerType.PathStat, error) {
	return d.apiClient.ContainerStatPath(ctx, container, path)
}

func (d *DryRunClient) ContainerStats(ctx context.Context, container string, stream bool) (containerType.StatsResponseReader, error) {
	return d.apiClient.ContainerStats(ctx, container, stream)
}

func (d *DryRunClient) ContainerStatsOneShot(ctx context.Context, container string) (containerType.StatsResponseReader, error) {
	return d.apiClient.ContainerStatsOneShot(ctx, container)
}

func (d *DryRunClient) ContainerTop(ctx context.Context, container string, arguments []string) (containerType.ContainerTopOKBody, error) {
	return d.apiClient.ContainerTop(ctx, container, arguments)
}

func (d *DryRunClient) ContainerUpdate(ctx context.Context, container string, updateConfig containerType.UpdateConfig) (containerType.ContainerUpdateOKBody, error) {
	return d.apiClient.ContainerUpdate(ctx, container, updateConfig)
}

func (d *DryRunClient) ContainerWait(ctx context.Context, container string, condition containerType.WaitCondition) (<-chan containerType.WaitResponse, <-chan error) {
	return d.apiClient.ContainerWait(ctx, container, condition)
}

func (d *DryRunClient) ContainersPrune(ctx context.Context, pruneFilters filters.Args) (containerType.PruneReport, error) {
	return d.apiClient.ContainersPrune(ctx, pruneFilters)
}

func (d *DryRunClient) DistributionInspect(ctx context.Context, imageName, encodedRegistryAuth string) (registry.DistributionInspect, error) {
	return d.apiClient.DistributionInspect(ctx, imageName, encodedRegistryAuth)
}

func (d *DryRunClient) BuildCachePrune(ctx context.Context, opts moby.BuildCachePruneOptions) (*moby.BuildCachePruneReport, error) {
	return d.apiClient.BuildCachePrune(ctx, opts)
}

func (d *DryRunClient) BuildCancel(ctx context.Context, id string) error {
	return d.apiClient.BuildCancel(ctx, id)
}

func (d *DryRunClient) ImageCreate(ctx context.Context, parentReference string, options image.CreateOptions) (io.ReadCloser, error) {
	return d.apiClient.ImageCreate(ctx, parentReference, options)
}

func (d *DryRunClient) ImageHistory(ctx context.Context, imageName string) ([]image.HistoryResponseItem, error) {
	return d.apiClient.ImageHistory(ctx, imageName)
}

func (d *DryRunClient) ImageImport(ctx context.Context, source image.ImportSource, ref string, options image.ImportOptions) (io.ReadCloser, error) {
	return d.apiClient.ImageImport(ctx, source, ref, options)
}

func (d *DryRunClient) ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
	return d.apiClient.ImageList(ctx, options)
}

func (d *DryRunClient) ImageLoad(ctx context.Context, input io.Reader, quiet bool) (image.LoadResponse, error) {
	return d.apiClient.ImageLoad(ctx, input, quiet)
}

func (d *DryRunClient) ImageSearch(ctx context.Context, term string, options registry.SearchOptions) ([]registry.SearchResult, error) {
	return d.apiClient.ImageSearch(ctx, term, options)
}

func (d *DryRunClient) ImageSave(ctx context.Context, images []string) (io.ReadCloser, error) {
	return d.apiClient.ImageSave(ctx, images)
}

func (d *DryRunClient) ImageTag(ctx context.Context, imageName, ref string) error {
	return d.apiClient.ImageTag(ctx, imageName, ref)
}

func (d *DryRunClient) ImagesPrune(ctx context.Context, pruneFilter filters.Args) (image.PruneReport, error) {
	return d.apiClient.ImagesPrune(ctx, pruneFilter)
}

func (d *DryRunClient) NodeInspectWithRaw(ctx context.Context, nodeID string) (swarm.Node, []byte, error) {
	return d.apiClient.NodeInspectWithRaw(ctx, nodeID)
}

func (d *DryRunClient) NodeList(ctx context.Context, options moby.NodeListOptions) ([]swarm.Node, error) {
	return d.apiClient.NodeList(ctx, options)
}

func (d *DryRunClient) NodeRemove(ctx context.Context, nodeID string, options moby.NodeRemoveOptions) error {
	return d.apiClient.NodeRemove(ctx, nodeID, options)
}

func (d *DryRunClient) NodeUpdate(ctx context.Context, nodeID string, version swarm.Version, node swarm.NodeSpec) error {
	return d.apiClient.NodeUpdate(ctx, nodeID, version, node)
}

func (d *DryRunClient) NetworkInspect(ctx context.Context, networkName string, options network.InspectOptions) (network.Inspect, error) {
	return d.apiClient.NetworkInspect(ctx, networkName, options)
}

func (d *DryRunClient) NetworkInspectWithRaw(ctx context.Context, networkName string, options network.InspectOptions) (network.Inspect, []byte, error) {
	return d.apiClient.NetworkInspectWithRaw(ctx, networkName, options)
}

func (d *DryRunClient) NetworkList(ctx context.Context, options network.ListOptions) ([]network.Inspect, error) {
	return d.apiClient.NetworkList(ctx, options)
}

func (d *DryRunClient) NetworksPrune(ctx context.Context, pruneFilter filters.Args) (network.PruneReport, error) {
	return d.apiClient.NetworksPrune(ctx, pruneFilter)
}

func (d *DryRunClient) PluginList(ctx context.Context, filter filters.Args) (moby.PluginsListResponse, error) {
	return d.apiClient.PluginList(ctx, filter)
}

func (d *DryRunClient) PluginRemove(ctx context.Context, name string, options moby.PluginRemoveOptions) error {
	return d.apiClient.PluginRemove(ctx, name, options)
}

func (d *DryRunClient) PluginEnable(ctx context.Context, name string, options moby.PluginEnableOptions) error {
	return d.apiClient.PluginEnable(ctx, name, options)
}

func (d *DryRunClient) PluginDisable(ctx context.Context, name string, options moby.PluginDisableOptions) error {
	return d.apiClient.PluginDisable(ctx, name, options)
}

func (d *DryRunClient) PluginInstall(ctx context.Context, name string, options moby.PluginInstallOptions) (io.ReadCloser, error) {
	return d.apiClient.PluginInstall(ctx, name, options)
}

func (d *DryRunClient) PluginUpgrade(ctx context.Context, name string, options moby.PluginInstallOptions) (io.ReadCloser, error) {
	return d.apiClient.PluginUpgrade(ctx, name, options)
}

func (d *DryRunClient) PluginPush(ctx context.Context, name string, registryAuth string) (io.ReadCloser, error) {
	return d.apiClient.PluginPush(ctx, name, registryAuth)
}

func (d *DryRunClient) PluginSet(ctx context.Context, name string, args []string) error {
	return d.apiClient.PluginSet(ctx, name, args)
}

func (d *DryRunClient) PluginInspectWithRaw(ctx context.Context, name string) (*moby.Plugin, []byte, error) {
	return d.apiClient.PluginInspectWithRaw(ctx, name)
}

func (d *DryRunClient) PluginCreate(ctx context.Context, createContext io.Reader, options moby.PluginCreateOptions) error {
	return d.apiClient.PluginCreate(ctx, createContext, options)
}

func (d *DryRunClient) ServiceCreate(ctx context.Context, service swarm.ServiceSpec, options moby.ServiceCreateOptions) (swarm.ServiceCreateResponse, error) {
	return d.apiClient.ServiceCreate(ctx, service, options)
}

func (d *DryRunClient) ServiceInspectWithRaw(ctx context.Context, serviceID string, options moby.ServiceInspectOptions) (swarm.Service, []byte, error) {
	return d.apiClient.ServiceInspectWithRaw(ctx, serviceID, options)
}

func (d *DryRunClient) ServiceList(ctx context.Context, options moby.ServiceListOptions) ([]swarm.Service, error) {
	return d.apiClient.ServiceList(ctx, options)
}

func (d *DryRunClient) ServiceRemove(ctx context.Context, serviceID string) error {
	return d.apiClient.ServiceRemove(ctx, serviceID)
}

func (d *DryRunClient) ServiceUpdate(ctx context.Context, serviceID string, version swarm.Version, service swarm.ServiceSpec, options moby.ServiceUpdateOptions) (swarm.ServiceUpdateResponse, error) {
	return d.apiClient.ServiceUpdate(ctx, serviceID, version, service, options)
}

func (d *DryRunClient) ServiceLogs(ctx context.Context, serviceID string, options containerType.LogsOptions) (io.ReadCloser, error) {
	return d.apiClient.ServiceLogs(ctx, serviceID, options)
}

func (d *DryRunClient) TaskLogs(ctx context.Context, taskID string, options containerType.LogsOptions) (io.ReadCloser, error) {
	return d.apiClient.TaskLogs(ctx, taskID, options)
}

func (d *DryRunClient) TaskInspectWithRaw(ctx context.Context, taskID string) (swarm.Task, []byte, error) {
	return d.apiClient.TaskInspectWithRaw(ctx, taskID)
}

func (d *DryRunClient) TaskList(ctx context.Context, options moby.TaskListOptions) ([]swarm.Task, error) {
	return d.apiClient.TaskList(ctx, options)
}

func (d *DryRunClient) SwarmInit(ctx context.Context, req swarm.InitRequest) (string, error) {
	return d.apiClient.SwarmInit(ctx, req)
}

func (d *DryRunClient) SwarmJoin(ctx context.Context, req swarm.JoinRequest) error {
	return d.apiClient.SwarmJoin(ctx, req)
}

func (d *DryRunClient) SwarmGetUnlockKey(ctx context.Context) (moby.SwarmUnlockKeyResponse, error) {
	return d.apiClient.SwarmGetUnlockKey(ctx)
}

func (d *DryRunClient) SwarmUnlock(ctx context.Context, req swarm.UnlockRequest) error {
	return d.apiClient.SwarmUnlock(ctx, req)
}

func (d *DryRunClient) SwarmLeave(ctx context.Context, force bool) error {
	return d.apiClient.SwarmLeave(ctx, force)
}

func (d *DryRunClient) SwarmInspect(ctx context.Context) (swarm.Swarm, error) {
	return d.apiClient.SwarmInspect(ctx)
}

func (d *DryRunClient) SwarmUpdate(ctx context.Context, version swarm.Version, swarmSpec swarm.Spec, flags swarm.UpdateFlags) error {
	return d.apiClient.SwarmUpdate(ctx, version, swarmSpec, flags)
}

func (d *DryRunClient) SecretList(ctx context.Context, options moby.SecretListOptions) ([]swarm.Secret, error) {
	return d.apiClient.SecretList(ctx, options)
}

func (d *DryRunClient) SecretCreate(ctx context.Context, secret swarm.SecretSpec) (moby.SecretCreateResponse, error) {
	return d.apiClient.SecretCreate(ctx, secret)
}

func (d *DryRunClient) SecretRemove(ctx context.Context, id string) error {
	return d.apiClient.SecretRemove(ctx, id)
}

func (d *DryRunClient) SecretInspectWithRaw(ctx context.Context, name string) (swarm.Secret, []byte, error) {
	return d.apiClient.SecretInspectWithRaw(ctx, name)
}

func (d *DryRunClient) SecretUpdate(ctx context.Context, id string, version swarm.Version, secret swarm.SecretSpec) error {
	return d.apiClient.SecretUpdate(ctx, id, version, secret)
}

func (d *DryRunClient) Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
	return d.apiClient.Events(ctx, options)
}

func (d *DryRunClient) Info(ctx context.Context) (system.Info, error) {
	return d.apiClient.Info(ctx)
}

func (d *DryRunClient) RegistryLogin(ctx context.Context, auth registry.AuthConfig) (registry.AuthenticateOKBody, error) {
	return d.apiClient.RegistryLogin(ctx, auth)
}

func (d *DryRunClient) DiskUsage(ctx context.Context, options moby.DiskUsageOptions) (moby.DiskUsage, error) {
	return d.apiClient.DiskUsage(ctx, options)
}

func (d *DryRunClient) Ping(ctx context.Context) (moby.Ping, error) {
	return d.apiClient.Ping(ctx)
}

func (d *DryRunClient) VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error) {
	return d.apiClient.VolumeInspect(ctx, volumeID)
}

func (d *DryRunClient) VolumeInspectWithRaw(ctx context.Context, volumeID string) (volume.Volume, []byte, error) {
	return d.apiClient.VolumeInspectWithRaw(ctx, volumeID)
}

func (d *DryRunClient) VolumeList(ctx context.Context, opts volume.ListOptions) (volume.ListResponse, error) {
	return d.apiClient.VolumeList(ctx, opts)
}

func (d *DryRunClient) VolumesPrune(ctx context.Context, pruneFilter filters.Args) (volume.PruneReport, error) {
	return d.apiClient.VolumesPrune(ctx, pruneFilter)
}

func (d *DryRunClient) VolumeUpdate(ctx context.Context, volumeID string, version swarm.Version, options volume.UpdateOptions) error {
	return d.apiClient.VolumeUpdate(ctx, volumeID, version, options)
}

func (d *DryRunClient) ClientVersion() string {
	return d.apiClient.ClientVersion()
}

func (d *DryRunClient) DaemonHost() string {
	return d.apiClient.DaemonHost()
}

func (d *DryRunClient) HTTPClient() *http.Client {
	return d.apiClient.HTTPClient()
}

func (d *DryRunClient) ServerVersion(ctx context.Context) (moby.Version, error) {
	return d.apiClient.ServerVersion(ctx)
}

func (d *DryRunClient) NegotiateAPIVersion(ctx context.Context) {
	d.apiClient.NegotiateAPIVersion(ctx)
}

func (d *DryRunClient) NegotiateAPIVersionPing(ping moby.Ping) {
	d.apiClient.NegotiateAPIVersionPing(ping)
}

func (d *DryRunClient) DialHijack(ctx context.Context, url, proto string, meta map[string][]string) (net.Conn, error) {
	return d.apiClient.DialHijack(ctx, url, proto, meta)
}

func (d *DryRunClient) Dialer() func(context.Context) (net.Conn, error) {
	return d.apiClient.Dialer()
}

func (d *DryRunClient) Close() error {
	return d.apiClient.Close()
}

func (d *DryRunClient) CheckpointCreate(ctx context.Context, container string, options checkpoint.CreateOptions) error {
	return d.apiClient.CheckpointCreate(ctx, container, options)
}

func (d *DryRunClient) CheckpointDelete(ctx context.Context, container string, options checkpoint.DeleteOptions) error {
	return d.apiClient.CheckpointDelete(ctx, container, options)
}

func (d *DryRunClient) CheckpointList(ctx context.Context, container string, options checkpoint.ListOptions) ([]checkpoint.Summary, error) {
	return d.apiClient.CheckpointList(ctx, container, options)
}
