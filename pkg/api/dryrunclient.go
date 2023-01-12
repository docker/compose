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
	"context"
	"io"
	"net"
	"net/http"

	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

var _ client.APIClient = &DryRunClient{}

// DryRunClient implements APIClient by delegating to implementation functions. This allows lazy init and per-method overrides
type DryRunClient struct {
	CopyFromContainerFn    func(ctx context.Context, container, srcPath string) (io.ReadCloser, moby.ContainerPathStat, error)
	CopyToContainerFn      func(ctx context.Context, container, path string, content io.Reader, options moby.CopyToContainerOptions) error
	ContainersPruneFn      func(ctx context.Context, pruneFilters filters.Args) (moby.ContainersPruneReport, error)
	ConfigListFn           func(ctx context.Context, options moby.ConfigListOptions) ([]swarm.Config, error)
	ConfigCreateFn         func(ctx context.Context, config swarm.ConfigSpec) (moby.ConfigCreateResponse, error)
	ConfigRemoveFn         func(ctx context.Context, id string) error
	ConfigInspectWithRawFn func(ctx context.Context, name string) (swarm.Config, []byte, error)
	ConfigUpdateFn         func(ctx context.Context, id string, version swarm.Version, config swarm.ConfigSpec) error
	ContainerAttachFn      func(ctx context.Context, container string, options moby.ContainerAttachOptions) (moby.HijackedResponse, error)
	ContainerCommitFn      func(ctx context.Context, container string, options moby.ContainerCommitOptions) (moby.IDResponse, error)
	ContainerCreateFn      func(ctx context.Context, config *containerType.Config, hostConfig *containerType.HostConfig,
		networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (containerType.CreateResponse, error)
	ContainerDiffFn           func(ctx context.Context, container string) ([]containerType.ContainerChangeResponseItem, error)
	ContainerExecAttachFn     func(ctx context.Context, execID string, config moby.ExecStartCheck) (moby.HijackedResponse, error)
	ContainerExecCreateFn     func(ctx context.Context, container string, config moby.ExecConfig) (moby.IDResponse, error)
	ContainerExecInspectFn    func(ctx context.Context, execID string) (moby.ContainerExecInspect, error)
	ContainerExecResizeFn     func(ctx context.Context, execID string, options moby.ResizeOptions) error
	ContainerExecStartFn      func(ctx context.Context, execID string, config moby.ExecStartCheck) error
	ContainerExportFn         func(ctx context.Context, container string) (io.ReadCloser, error)
	ContainerInspectFn        func(ctx context.Context, container string) (moby.ContainerJSON, error)
	ContainerInspectWithRawFn func(ctx context.Context, container string, getSize bool) (moby.ContainerJSON, []byte, error)
	ContainerKillFn           func(ctx context.Context, container, signal string) error
	ContainerListFn           func(ctx context.Context, options moby.ContainerListOptions) ([]moby.Container, error)
	ContainerLogsFn           func(ctx context.Context, container string, options moby.ContainerLogsOptions) (io.ReadCloser, error)
	ContainerPauseFn          func(ctx context.Context, container string) error
	ContainerRemoveFn         func(ctx context.Context, container string, options moby.ContainerRemoveOptions) error
	ContainerRenameFn         func(ctx context.Context, container, newContainerName string) error
	ContainerResizeFn         func(ctx context.Context, container string, options moby.ResizeOptions) error
	ContainerRestartFn        func(ctx context.Context, container string, options containerType.StopOptions) error
	ContainerStatPathFn       func(ctx context.Context, container, path string) (moby.ContainerPathStat, error)
	ContainerStatsFn          func(ctx context.Context, container string, stream bool) (moby.ContainerStats, error)
	ContainerStatsOneShotFn   func(ctx context.Context, container string) (moby.ContainerStats, error)
	ContainerStartFn          func(ctx context.Context, container string, options moby.ContainerStartOptions) error
	ContainerStopFn           func(ctx context.Context, container string, options containerType.StopOptions) error
	ContainerTopFn            func(ctx context.Context, container string, arguments []string) (containerType.ContainerTopOKBody, error)
	ContainerUnpauseFn        func(ctx context.Context, container string) error
	ContainerUpdateFn         func(ctx context.Context, container string, updateConfig containerType.UpdateConfig) (containerType.ContainerUpdateOKBody, error)
	ContainerWaitFn           func(ctx context.Context, container string, condition containerType.WaitCondition) (<-chan containerType.WaitResponse, <-chan error)
	DistributionInspectFn     func(ctx context.Context, imageName, encodedRegistryAuth string) (registry.DistributionInspect, error)
	ImageBuildFn              func(ctx context.Context, reader io.Reader, options moby.ImageBuildOptions) (moby.ImageBuildResponse, error)
	BuildCachePruneFn         func(ctx context.Context, opts moby.BuildCachePruneOptions) (*moby.BuildCachePruneReport, error)
	BuildCancelFn             func(ctx context.Context, id string) error
	ImageCreateFn             func(ctx context.Context, parentReference string, options moby.ImageCreateOptions) (io.ReadCloser, error)
	ImageHistoryFn            func(ctx context.Context, imageName string) ([]image.HistoryResponseItem, error)
	ImageImportFn             func(ctx context.Context, source moby.ImageImportSource, ref string, options moby.ImageImportOptions) (io.ReadCloser, error)
	ImageInspectWithRawFn     func(ctx context.Context, imageName string) (moby.ImageInspect, []byte, error)
	ImageListFn               func(ctx context.Context, options moby.ImageListOptions) ([]moby.ImageSummary, error)
	ImageLoadFn               func(ctx context.Context, input io.Reader, quiet bool) (moby.ImageLoadResponse, error)
	ImagePullFn               func(ctx context.Context, ref string, options moby.ImagePullOptions) (io.ReadCloser, error)
	ImagePushFn               func(ctx context.Context, ref string, options moby.ImagePushOptions) (io.ReadCloser, error)
	ImageRemoveFn             func(ctx context.Context, image string, options moby.ImageRemoveOptions) ([]moby.ImageDeleteResponseItem, error)
	ImageSearchFn             func(ctx context.Context, term string, options moby.ImageSearchOptions) ([]registry.SearchResult, error)
	ImageSaveFn               func(ctx context.Context, images []string) (io.ReadCloser, error)
	ImageTagFn                func(ctx context.Context, image, ref string) error
	ImagesPruneFn             func(ctx context.Context, pruneFilter filters.Args) (moby.ImagesPruneReport, error)
	NodeInspectWithRawFn      func(ctx context.Context, nodeID string) (swarm.Node, []byte, error)
	NodeListFn                func(ctx context.Context, options moby.NodeListOptions) ([]swarm.Node, error)
	NodeRemoveFn              func(ctx context.Context, nodeID string, options moby.NodeRemoveOptions) error
	NodeUpdateFn              func(ctx context.Context, nodeID string, version swarm.Version, node swarm.NodeSpec) error
	NetworkConnectFn          func(ctx context.Context, network, container string, config *network.EndpointSettings) error
	NetworkCreateFn           func(ctx context.Context, name string, options moby.NetworkCreate) (moby.NetworkCreateResponse, error)
	NetworkDisconnectFn       func(ctx context.Context, network, container string, force bool) error
	NetworkInspectFn          func(ctx context.Context, network string, options moby.NetworkInspectOptions) (moby.NetworkResource, error)
	NetworkInspectWithRawFn   func(ctx context.Context, network string, options moby.NetworkInspectOptions) (moby.NetworkResource, []byte, error)
	NetworkListFn             func(ctx context.Context, options moby.NetworkListOptions) ([]moby.NetworkResource, error)
	NetworkRemoveFn           func(ctx context.Context, network string) error
	NetworksPruneFn           func(ctx context.Context, pruneFilter filters.Args) (moby.NetworksPruneReport, error)
	PluginListFn              func(ctx context.Context, filter filters.Args) (moby.PluginsListResponse, error)
	PluginRemoveFn            func(ctx context.Context, name string, options moby.PluginRemoveOptions) error
	PluginEnableFn            func(ctx context.Context, name string, options moby.PluginEnableOptions) error
	PluginDisableFn           func(ctx context.Context, name string, options moby.PluginDisableOptions) error
	PluginInstallFn           func(ctx context.Context, name string, options moby.PluginInstallOptions) (io.ReadCloser, error)
	PluginUpgradeFn           func(ctx context.Context, name string, options moby.PluginInstallOptions) (io.ReadCloser, error)
	PluginPushFn              func(ctx context.Context, name string, registryAuth string) (io.ReadCloser, error)
	PluginSetFn               func(ctx context.Context, name string, args []string) error
	PluginInspectWithRawFn    func(ctx context.Context, name string) (*moby.Plugin, []byte, error)
	PluginCreateFn            func(ctx context.Context, createContext io.Reader, options moby.PluginCreateOptions) error
	ServiceCreateFn           func(ctx context.Context, service swarm.ServiceSpec, options moby.ServiceCreateOptions) (moby.ServiceCreateResponse, error)
	ServiceInspectWithRawFn   func(ctx context.Context, serviceID string, options moby.ServiceInspectOptions) (swarm.Service, []byte, error)
	ServiceListFn             func(ctx context.Context, options moby.ServiceListOptions) ([]swarm.Service, error)
	ServiceRemoveFn           func(ctx context.Context, serviceID string) error
	ServiceUpdateFn           func(ctx context.Context, serviceID string, version swarm.Version, service swarm.ServiceSpec, options moby.ServiceUpdateOptions) (moby.ServiceUpdateResponse, error)
	ServiceLogsFn             func(ctx context.Context, serviceID string, options moby.ContainerLogsOptions) (io.ReadCloser, error)
	TaskLogsFn                func(ctx context.Context, taskID string, options moby.ContainerLogsOptions) (io.ReadCloser, error)
	TaskInspectWithRawFn      func(ctx context.Context, taskID string) (swarm.Task, []byte, error)
	TaskListFn                func(ctx context.Context, options moby.TaskListOptions) ([]swarm.Task, error)
	SwarmInitFn               func(ctx context.Context, req swarm.InitRequest) (string, error)
	SwarmJoinFn               func(ctx context.Context, req swarm.JoinRequest) error
	SwarmGetUnlockKeyFn       func(ctx context.Context) (moby.SwarmUnlockKeyResponse, error)
	SwarmUnlockFn             func(ctx context.Context, req swarm.UnlockRequest) error
	SwarmLeaveFn              func(ctx context.Context, force bool) error
	SwarmInspectFn            func(ctx context.Context) (swarm.Swarm, error)
	SwarmUpdateFn             func(ctx context.Context, version swarm.Version, swarm swarm.Spec, flags swarm.UpdateFlags) error
	SecretListFn              func(ctx context.Context, options moby.SecretListOptions) ([]swarm.Secret, error)
	SecretCreateFn            func(ctx context.Context, secret swarm.SecretSpec) (moby.SecretCreateResponse, error)
	SecretRemoveFn            func(ctx context.Context, id string) error
	SecretInspectWithRawFn    func(ctx context.Context, name string) (swarm.Secret, []byte, error)
	SecretUpdateFn            func(ctx context.Context, id string, version swarm.Version, secret swarm.SecretSpec) error
	EventsFn                  func(ctx context.Context, options moby.EventsOptions) (<-chan events.Message, <-chan error)
	InfoFn                    func(ctx context.Context) (moby.Info, error)
	RegistryLoginFn           func(ctx context.Context, auth moby.AuthConfig) (registry.AuthenticateOKBody, error)
	DiskUsageFn               func(ctx context.Context, options moby.DiskUsageOptions) (moby.DiskUsage, error)
	PingFn                    func(ctx context.Context) (moby.Ping, error)
	VolumeCreateFn            func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeInspectFn           func(ctx context.Context, volumeID string) (volume.Volume, error)
	VolumeInspectWithRawFn    func(ctx context.Context, volumeID string) (volume.Volume, []byte, error)
	VolumeListFn              func(ctx context.Context, filter filters.Args) (volume.ListResponse, error)
	VolumeRemoveFn            func(ctx context.Context, volumeID string, force bool) error
	VolumesPruneFn            func(ctx context.Context, pruneFilter filters.Args) (moby.VolumesPruneReport, error)
	VolumeUpdateFn            func(ctx context.Context, volumeID string, version swarm.Version, options volume.UpdateOptions) error
	ClientVersionFn           func() string
	DaemonHostFn              func() string
	HTTPClientFn              func() *http.Client
	ServerVersionFn           func(ctx context.Context) (moby.Version, error)
	NegotiateAPIVersionFn     func(ctx context.Context)
	NegotiateAPIVersionPingFn func(ping moby.Ping)
	DialHijackFn              func(ctx context.Context, url, proto string, meta map[string][]string) (net.Conn, error)
	DialerFn                  func() func(context.Context) (net.Conn, error)
	CloseFn                   func() error
	CheckpointCreateFn        func(ctx context.Context, container string, options moby.CheckpointCreateOptions) error
	CheckpointDeleteFn        func(ctx context.Context, container string, options moby.CheckpointDeleteOptions) error
	CheckpointListFn          func(ctx context.Context, container string, options moby.CheckpointListOptions) ([]moby.Checkpoint, error)
}

// NewDryRunClient produces a DryRunClient
func NewDryRunClient() *DryRunClient {
	return &DryRunClient{}
}

// WithAPIClient configure DryRunClient to use specified APIClient as delegate
func (d *DryRunClient) WithAPIClient(apiClient client.APIClient) {
	d.ConfigListFn = apiClient.ConfigList
	d.ConfigCreateFn = apiClient.ConfigCreate
	d.ConfigRemoveFn = apiClient.ConfigRemove
	d.ConfigInspectWithRawFn = apiClient.ConfigInspectWithRaw
	d.ConfigUpdateFn = apiClient.ConfigUpdate
	d.ContainerAttachFn = apiClient.ContainerAttach
	d.ContainerCommitFn = apiClient.ContainerCommit
	d.ContainerCreateFn = apiClient.ContainerCreate
	d.ContainerDiffFn = apiClient.ContainerDiff
	d.ContainerExecAttachFn = apiClient.ContainerExecAttach
	d.ContainerExecCreateFn = apiClient.ContainerExecCreate
	d.ContainerExecInspectFn = apiClient.ContainerExecInspect
	d.ContainerExecResizeFn = apiClient.ContainerExecResize
	d.ContainerExecStartFn = apiClient.ContainerExecStart
	d.ContainerExportFn = apiClient.ContainerExport
	d.ContainerInspectFn = apiClient.ContainerInspect
	d.ContainerInspectWithRawFn = apiClient.ContainerInspectWithRaw
	d.ContainerKillFn = apiClient.ContainerKill
	d.ContainerListFn = apiClient.ContainerList
	d.ContainerLogsFn = apiClient.ContainerLogs
	d.ContainerPauseFn = apiClient.ContainerPause
	d.ContainerRemoveFn = apiClient.ContainerRemove
	d.ContainerRenameFn = apiClient.ContainerRename
	d.ContainerResizeFn = apiClient.ContainerResize
	d.ContainerRestartFn = apiClient.ContainerRestart
	d.ContainerStatPathFn = apiClient.ContainerStatPath
	d.ContainerStatsFn = apiClient.ContainerStats
	d.ContainerStatsOneShotFn = apiClient.ContainerStatsOneShot
	d.ContainerStartFn = apiClient.ContainerStart
	d.ContainerStopFn = apiClient.ContainerStop
	d.ContainerTopFn = apiClient.ContainerTop
	d.ContainerUnpauseFn = apiClient.ContainerUnpause
	d.ContainerUpdateFn = apiClient.ContainerUpdate
	d.ContainerWaitFn = apiClient.ContainerWait
	d.DistributionInspectFn = apiClient.DistributionInspect
	d.ImageBuildFn = apiClient.ImageBuild
	d.BuildCachePruneFn = apiClient.BuildCachePrune
	d.BuildCancelFn = apiClient.BuildCancel
	d.ImageCreateFn = apiClient.ImageCreate
	d.ImageHistoryFn = apiClient.ImageHistory
	d.ImageImportFn = apiClient.ImageImport
	d.ImageInspectWithRawFn = apiClient.ImageInspectWithRaw
	d.ImageListFn = apiClient.ImageList
	d.ImageLoadFn = apiClient.ImageLoad
	d.ImagePullFn = apiClient.ImagePull
	d.ImagePushFn = apiClient.ImagePush
	d.ImageRemoveFn = apiClient.ImageRemove
	d.ImageSearchFn = apiClient.ImageSearch
	d.ImageSaveFn = apiClient.ImageSave
	d.ImageTagFn = apiClient.ImageTag
	d.ImagesPruneFn = apiClient.ImagesPrune
	d.NodeInspectWithRawFn = apiClient.NodeInspectWithRaw
	d.NodeListFn = apiClient.NodeList
	d.NodeRemoveFn = apiClient.NodeRemove
	d.NodeUpdateFn = apiClient.NodeUpdate
	d.NetworkConnectFn = apiClient.NetworkConnect
	d.NetworkCreateFn = apiClient.NetworkCreate
	d.NetworkDisconnectFn = apiClient.NetworkDisconnect
	d.NetworkInspectFn = apiClient.NetworkInspect
	d.NetworkInspectWithRawFn = apiClient.NetworkInspectWithRaw
	d.NetworkListFn = apiClient.NetworkList
	d.NetworkRemoveFn = apiClient.NetworkRemove
	d.NetworksPruneFn = apiClient.NetworksPrune
	d.PluginListFn = apiClient.PluginList
	d.PluginRemoveFn = apiClient.PluginRemove
	d.PluginEnableFn = apiClient.PluginEnable
	d.PluginDisableFn = apiClient.PluginDisable
	d.PluginInstallFn = apiClient.PluginInstall
	d.PluginUpgradeFn = apiClient.PluginUpgrade
	d.PluginPushFn = apiClient.PluginPush
	d.PluginSetFn = apiClient.PluginSet
	d.PluginInspectWithRawFn = apiClient.PluginInspectWithRaw
	d.PluginCreateFn = apiClient.PluginCreate
	d.ServiceCreateFn = apiClient.ServiceCreate
	d.ServiceInspectWithRawFn = apiClient.ServiceInspectWithRaw
	d.ServiceListFn = apiClient.ServiceList
	d.ServiceRemoveFn = apiClient.ServiceRemove
	d.ServiceUpdateFn = apiClient.ServiceUpdate
	d.ServiceLogsFn = apiClient.ServiceLogs
	d.TaskLogsFn = apiClient.TaskLogs
	d.TaskInspectWithRawFn = apiClient.TaskInspectWithRaw
	d.TaskListFn = apiClient.TaskList
	d.SwarmInitFn = apiClient.SwarmInit
	d.SwarmJoinFn = apiClient.SwarmJoin
	d.SwarmGetUnlockKeyFn = apiClient.SwarmGetUnlockKey
	d.SwarmUnlockFn = apiClient.SwarmUnlock
	d.SwarmLeaveFn = apiClient.SwarmLeave
	d.SwarmInspectFn = apiClient.SwarmInspect
	d.SwarmUpdateFn = apiClient.SwarmUpdate
	d.SecretListFn = apiClient.SecretList
	d.SecretCreateFn = apiClient.SecretCreate
	d.SecretRemoveFn = apiClient.SecretRemove
	d.SecretInspectWithRawFn = apiClient.SecretInspectWithRaw
	d.SecretUpdateFn = apiClient.SecretUpdate
	d.EventsFn = apiClient.Events
	d.InfoFn = apiClient.Info
	d.RegistryLoginFn = apiClient.RegistryLogin
	d.DiskUsageFn = apiClient.DiskUsage
	d.PingFn = apiClient.Ping
	d.VolumeCreateFn = apiClient.VolumeCreate
	d.VolumeInspectFn = apiClient.VolumeInspect
	d.VolumeInspectWithRawFn = apiClient.VolumeInspectWithRaw
	d.VolumeListFn = apiClient.VolumeList
	d.VolumeRemoveFn = apiClient.VolumeRemove
	d.VolumesPruneFn = apiClient.VolumesPrune
	d.VolumeUpdateFn = apiClient.VolumeUpdate
	d.ClientVersionFn = apiClient.ClientVersion
	d.DaemonHostFn = apiClient.DaemonHost
	d.HTTPClientFn = apiClient.HTTPClient
	d.ServerVersionFn = apiClient.ServerVersion
	d.NegotiateAPIVersionFn = apiClient.NegotiateAPIVersion
	d.NegotiateAPIVersionPingFn = apiClient.NegotiateAPIVersionPing
	d.DialHijackFn = apiClient.DialHijack
	d.DialerFn = apiClient.Dialer
	d.CloseFn = apiClient.Close
	d.CheckpointCreateFn = apiClient.CheckpointCreate
	d.CheckpointDeleteFn = apiClient.CheckpointDelete
	d.CheckpointListFn = apiClient.CheckpointList
}

func (d *DryRunClient) ConfigList(ctx context.Context, options moby.ConfigListOptions) ([]swarm.Config, error) {
	if d.ConfigListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ConfigListFn(ctx, options)
}

func (d *DryRunClient) ConfigCreate(ctx context.Context, config swarm.ConfigSpec) (moby.ConfigCreateResponse, error) {
	if d.ConfigCreateFn == nil {
		return moby.ConfigCreateResponse{}, ErrNotImplemented
	}
	return d.ConfigCreateFn(ctx, config)
}

func (d *DryRunClient) ConfigRemove(ctx context.Context, id string) error {
	if d.ConfigRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.ConfigRemoveFn(ctx, id)
}

func (d *DryRunClient) ConfigInspectWithRaw(ctx context.Context, name string) (swarm.Config, []byte, error) {
	if d.ConfigInspectWithRawFn == nil {
		return swarm.Config{}, nil, ErrNotImplemented
	}
	return d.ConfigInspectWithRawFn(ctx, name)
}

func (d *DryRunClient) ConfigUpdate(ctx context.Context, id string, version swarm.Version, config swarm.ConfigSpec) error {
	if d.ConfigUpdateFn == nil {
		return ErrNotImplemented
	}
	return d.ConfigUpdateFn(ctx, id, version, config)
}

func (d *DryRunClient) ContainerAttach(ctx context.Context, container string, options moby.ContainerAttachOptions) (moby.HijackedResponse, error) {
	if d.ContainerAttachFn == nil {
		return moby.HijackedResponse{}, ErrNotImplemented
	}
	return d.ContainerAttachFn(ctx, container, options)
}

func (d *DryRunClient) ContainerCommit(ctx context.Context, container string, options moby.ContainerCommitOptions) (moby.IDResponse, error) {
	if d.ContainerCommitFn == nil {
		return moby.IDResponse{}, ErrNotImplemented
	}
	return d.ContainerCommitFn(ctx, container, options)
}

func (d *DryRunClient) ContainerCreate(ctx context.Context, config *containerType.Config, hostConfig *containerType.HostConfig,
	networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (containerType.CreateResponse, error) {
	if d.ContainerCreateFn == nil {
		return containerType.CreateResponse{}, ErrNotImplemented
	}
	return d.ContainerCreateFn(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

func (d *DryRunClient) ContainerDiff(ctx context.Context, container string) ([]containerType.ContainerChangeResponseItem, error) {
	if d.ContainerDiffFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ContainerDiffFn(ctx, container)
}

func (d *DryRunClient) ContainerExecAttach(ctx context.Context, execID string, config moby.ExecStartCheck) (moby.HijackedResponse, error) {
	if d.ContainerExecAttachFn == nil {
		return moby.HijackedResponse{}, ErrNotImplemented
	}
	return d.ContainerExecAttachFn(ctx, execID, config)
}

func (d *DryRunClient) ContainerExecCreate(ctx context.Context, container string, config moby.ExecConfig) (moby.IDResponse, error) {
	if d.ContainerExecCreateFn == nil {
		return moby.IDResponse{}, ErrNotImplemented
	}
	return d.ContainerExecCreateFn(ctx, container, config)
}

func (d *DryRunClient) ContainerExecInspect(ctx context.Context, execID string) (moby.ContainerExecInspect, error) {
	if d.ContainerExecInspectFn == nil {
		return moby.ContainerExecInspect{}, ErrNotImplemented
	}
	return d.ContainerExecInspectFn(ctx, execID)
}

func (d *DryRunClient) ContainerExecResize(ctx context.Context, execID string, options moby.ResizeOptions) error {
	if d.ContainerExecResizeFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerExecResizeFn(ctx, execID, options)
}

func (d *DryRunClient) ContainerExecStart(ctx context.Context, execID string, config moby.ExecStartCheck) error {
	if d.ContainerExecStartFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerExecStartFn(ctx, execID, config)
}

func (d *DryRunClient) ContainerExport(ctx context.Context, container string) (io.ReadCloser, error) {
	if d.ContainerExportFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ContainerExportFn(ctx, container)
}

func (d *DryRunClient) ContainerInspect(ctx context.Context, container string) (moby.ContainerJSON, error) {
	if d.ContainerInspectFn == nil {
		return moby.ContainerJSON{}, ErrNotImplemented
	}
	return d.ContainerInspectFn(ctx, container)
}

func (d *DryRunClient) ContainerInspectWithRaw(ctx context.Context, container string, getSize bool) (moby.ContainerJSON, []byte, error) {
	if d.ContainerInspectWithRawFn == nil {
		return moby.ContainerJSON{}, nil, ErrNotImplemented
	}
	return d.ContainerInspectWithRawFn(ctx, container, getSize)
}

func (d *DryRunClient) ContainerKill(ctx context.Context, container, signal string) error {
	if d.ContainerKillFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerKillFn(ctx, container, signal)
}

func (d *DryRunClient) ContainerList(ctx context.Context, options moby.ContainerListOptions) ([]moby.Container, error) {
	if d.ContainerListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ContainerListFn(ctx, options)
}

func (d *DryRunClient) ContainerLogs(ctx context.Context, container string, options moby.ContainerLogsOptions) (io.ReadCloser, error) {
	if d.ContainerLogsFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ContainerLogsFn(ctx, container, options)
}

func (d *DryRunClient) ContainerPause(ctx context.Context, container string) error {
	if d.ContainerPauseFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerPauseFn(ctx, container)
}

func (d *DryRunClient) ContainerRemove(ctx context.Context, container string, options moby.ContainerRemoveOptions) error {
	if d.ContainerRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerRemoveFn(ctx, container, options)
}

func (d *DryRunClient) ContainerRename(ctx context.Context, container, newContainerName string) error {
	if d.ContainerRenameFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerRenameFn(ctx, container, newContainerName)
}

func (d *DryRunClient) ContainerResize(ctx context.Context, container string, options moby.ResizeOptions) error {
	if d.ContainerResizeFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerResizeFn(ctx, container, options)
}

func (d *DryRunClient) ContainerRestart(ctx context.Context, container string, options containerType.StopOptions) error {
	if d.ContainerRestartFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerRestartFn(ctx, container, options)
}

func (d *DryRunClient) ContainerStatPath(ctx context.Context, container, path string) (moby.ContainerPathStat, error) {
	if d.ContainerStatPathFn == nil {
		return moby.ContainerPathStat{}, ErrNotImplemented
	}
	return d.ContainerStatPathFn(ctx, container, path)
}

func (d *DryRunClient) ContainerStats(ctx context.Context, container string, stream bool) (moby.ContainerStats, error) {
	if d.ContainerStatsFn == nil {
		return moby.ContainerStats{}, ErrNotImplemented
	}
	return d.ContainerStatsFn(ctx, container, stream)
}

func (d *DryRunClient) ContainerStatsOneShot(ctx context.Context, container string) (moby.ContainerStats, error) {
	if d.ContainerStatsOneShotFn == nil {
		return moby.ContainerStats{}, ErrNotImplemented
	}
	return d.ContainerStatsOneShotFn(ctx, container)
}

func (d *DryRunClient) ContainerStart(ctx context.Context, container string, options moby.ContainerStartOptions) error {
	if d.ContainerStartFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerStartFn(ctx, container, options)
}

func (d *DryRunClient) ContainerStop(ctx context.Context, container string, options containerType.StopOptions) error {
	if d.ContainerStopFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerStopFn(ctx, container, options)
}

func (d *DryRunClient) ContainerTop(ctx context.Context, container string, arguments []string) (containerType.ContainerTopOKBody, error) {
	if d.ContainerTopFn == nil {
		return containerType.ContainerTopOKBody{}, ErrNotImplemented
	}
	return d.ContainerTopFn(ctx, container, arguments)
}

func (d *DryRunClient) ContainerUnpause(ctx context.Context, container string) error {
	if d.ContainerUnpauseFn == nil {
		return ErrNotImplemented
	}
	return d.ContainerUnpauseFn(ctx, container)
}

func (d *DryRunClient) ContainerUpdate(ctx context.Context, container string, updateConfig containerType.UpdateConfig) (containerType.ContainerUpdateOKBody, error) {
	if d.ContainerUpdateFn == nil {
		return containerType.ContainerUpdateOKBody{}, ErrNotImplemented
	}
	return d.ContainerUpdateFn(ctx, container, updateConfig)
}

func (d *DryRunClient) ContainerWait(ctx context.Context, container string, condition containerType.WaitCondition) (<-chan containerType.WaitResponse, <-chan error) {
	if d.ContainerWaitFn == nil {
		errC := make(chan error, 1)
		errC <- ErrNotImplemented
		return nil, errC
	}
	return d.ContainerWaitFn(ctx, container, condition)
}

func (d *DryRunClient) CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, moby.ContainerPathStat, error) {
	if d.CopyFromContainerFn == nil {
		return nil, moby.ContainerPathStat{}, ErrNotImplemented
	}
	return d.CopyFromContainerFn(ctx, container, srcPath)
}

func (d *DryRunClient) CopyToContainer(ctx context.Context, container, path string, content io.Reader, options moby.CopyToContainerOptions) error {
	if d.CopyToContainerFn == nil {
		return ErrNotImplemented
	}
	return d.CopyToContainerFn(ctx, container, path, content, options)
}

func (d *DryRunClient) ContainersPrune(ctx context.Context, pruneFilters filters.Args) (moby.ContainersPruneReport, error) {
	if d.ContainersPruneFn == nil {
		return moby.ContainersPruneReport{}, ErrNotImplemented
	}
	return d.ContainersPruneFn(ctx, pruneFilters)
}

func (d *DryRunClient) DistributionInspect(ctx context.Context, imageName, encodedRegistryAuth string) (registry.DistributionInspect, error) {
	if d.DistributionInspectFn == nil {
		return registry.DistributionInspect{}, ErrNotImplemented
	}
	return d.DistributionInspectFn(ctx, imageName, encodedRegistryAuth)
}

func (d *DryRunClient) ImageBuild(ctx context.Context, reader io.Reader, options moby.ImageBuildOptions) (moby.ImageBuildResponse, error) {
	if d.ImageBuildFn == nil {
		return moby.ImageBuildResponse{}, ErrNotImplemented
	}
	return d.ImageBuildFn(ctx, reader, options)
}

func (d *DryRunClient) BuildCachePrune(ctx context.Context, opts moby.BuildCachePruneOptions) (*moby.BuildCachePruneReport, error) {
	if d.BuildCachePruneFn == nil {
		return nil, ErrNotImplemented
	}
	return d.BuildCachePruneFn(ctx, opts)
}

func (d *DryRunClient) BuildCancel(ctx context.Context, id string) error {
	if d.BuildCancelFn == nil {
		return ErrNotImplemented
	}
	return d.BuildCancelFn(ctx, id)
}

func (d *DryRunClient) ImageCreate(ctx context.Context, parentReference string, options moby.ImageCreateOptions) (io.ReadCloser, error) {
	if d.ImageCreateFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageCreateFn(ctx, parentReference, options)
}

func (d *DryRunClient) ImageHistory(ctx context.Context, imageName string) ([]image.HistoryResponseItem, error) {
	if d.ImageHistoryFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageHistoryFn(ctx, imageName)
}

func (d *DryRunClient) ImageImport(ctx context.Context, source moby.ImageImportSource, ref string, options moby.ImageImportOptions) (io.ReadCloser, error) {
	if d.ImageImportFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageImportFn(ctx, source, ref, options)
}

func (d *DryRunClient) ImageInspectWithRaw(ctx context.Context, imageName string) (moby.ImageInspect, []byte, error) {
	if d.ImageInspectWithRawFn == nil {
		return moby.ImageInspect{}, nil, ErrNotImplemented
	}
	return d.ImageInspectWithRawFn(ctx, imageName)
}

func (d *DryRunClient) ImageList(ctx context.Context, options moby.ImageListOptions) ([]moby.ImageSummary, error) {
	if d.ImageListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageListFn(ctx, options)
}

func (d *DryRunClient) ImageLoad(ctx context.Context, input io.Reader, quiet bool) (moby.ImageLoadResponse, error) {
	if d.ImageLoadFn == nil {
		return moby.ImageLoadResponse{}, ErrNotImplemented
	}
	return d.ImageLoadFn(ctx, input, quiet)
}

func (d *DryRunClient) ImagePull(ctx context.Context, ref string, options moby.ImagePullOptions) (io.ReadCloser, error) {
	if d.ImagePullFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImagePullFn(ctx, ref, options)
}

func (d *DryRunClient) ImagePush(ctx context.Context, ref string, options moby.ImagePushOptions) (io.ReadCloser, error) {
	if d.ImagePushFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImagePushFn(ctx, ref, options)
}

func (d *DryRunClient) ImageRemove(ctx context.Context, imageName string, options moby.ImageRemoveOptions) ([]moby.ImageDeleteResponseItem, error) {
	if d.ImageRemoveFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageRemoveFn(ctx, imageName, options)
}

func (d *DryRunClient) ImageSearch(ctx context.Context, term string, options moby.ImageSearchOptions) ([]registry.SearchResult, error) {
	if d.ImageSearchFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageSearchFn(ctx, term, options)
}

func (d *DryRunClient) ImageSave(ctx context.Context, images []string) (io.ReadCloser, error) {
	if d.ImageSaveFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ImageSaveFn(ctx, images)
}

func (d *DryRunClient) ImageTag(ctx context.Context, imageName, ref string) error {
	if d.ImageTagFn == nil {
		return ErrNotImplemented
	}
	return d.ImageTagFn(ctx, imageName, ref)
}

func (d *DryRunClient) ImagesPrune(ctx context.Context, pruneFilter filters.Args) (moby.ImagesPruneReport, error) {
	if d.ImagesPruneFn == nil {
		return moby.ImagesPruneReport{}, ErrNotImplemented
	}
	return d.ImagesPruneFn(ctx, pruneFilter)
}

func (d *DryRunClient) NodeInspectWithRaw(ctx context.Context, nodeID string) (swarm.Node, []byte, error) {
	if d.NodeInspectWithRawFn == nil {
		return swarm.Node{}, nil, ErrNotImplemented
	}
	return d.NodeInspectWithRawFn(ctx, nodeID)
}

func (d *DryRunClient) NodeList(ctx context.Context, options moby.NodeListOptions) ([]swarm.Node, error) {
	if d.NodeListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.NodeListFn(ctx, options)
}

func (d *DryRunClient) NodeRemove(ctx context.Context, nodeID string, options moby.NodeRemoveOptions) error {
	if d.NodeRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.NodeRemoveFn(ctx, nodeID, options)
}

func (d *DryRunClient) NodeUpdate(ctx context.Context, nodeID string, version swarm.Version, node swarm.NodeSpec) error {
	if d.NodeUpdateFn == nil {
		return ErrNotImplemented
	}
	return d.NodeUpdateFn(ctx, nodeID, version, node)
}

func (d *DryRunClient) NetworkConnect(ctx context.Context, networkName, container string, config *network.EndpointSettings) error {
	if d.NetworkConnectFn == nil {
		return ErrNotImplemented
	}
	return d.NetworkConnectFn(ctx, networkName, container, config)
}

func (d *DryRunClient) NetworkCreate(ctx context.Context, name string, options moby.NetworkCreate) (moby.NetworkCreateResponse, error) {
	if d.NetworkCreateFn == nil {
		return moby.NetworkCreateResponse{}, ErrNotImplemented
	}
	return d.NetworkCreateFn(ctx, name, options)
}

func (d *DryRunClient) NetworkDisconnect(ctx context.Context, networkName, container string, force bool) error {
	if d.NetworkDisconnectFn == nil {
		return ErrNotImplemented
	}
	return d.NetworkDisconnectFn(ctx, networkName, container, force)
}

func (d *DryRunClient) NetworkInspect(ctx context.Context, networkName string, options moby.NetworkInspectOptions) (moby.NetworkResource, error) {
	if d.NetworkInspectFn == nil {
		return moby.NetworkResource{}, ErrNotImplemented
	}
	return d.NetworkInspectFn(ctx, networkName, options)
}

func (d *DryRunClient) NetworkInspectWithRaw(ctx context.Context, networkName string, options moby.NetworkInspectOptions) (moby.NetworkResource, []byte, error) {
	if d.NetworkInspectWithRawFn == nil {
		return moby.NetworkResource{}, nil, ErrNotImplemented
	}
	return d.NetworkInspectWithRawFn(ctx, networkName, options)
}

func (d *DryRunClient) NetworkList(ctx context.Context, options moby.NetworkListOptions) ([]moby.NetworkResource, error) {
	if d.NetworkListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.NetworkListFn(ctx, options)
}

func (d *DryRunClient) NetworkRemove(ctx context.Context, networkName string) error {
	return d.NetworkRemoveFn(ctx, networkName)
}

func (d *DryRunClient) NetworksPrune(ctx context.Context, pruneFilter filters.Args) (moby.NetworksPruneReport, error) {
	if d.NetworksPruneFn == nil {
		return moby.NetworksPruneReport{}, ErrNotImplemented
	}
	return d.NetworksPruneFn(ctx, pruneFilter)
}

func (d *DryRunClient) PluginList(ctx context.Context, filter filters.Args) (moby.PluginsListResponse, error) {
	if d.PluginListFn == nil {
		return moby.PluginsListResponse{}, ErrNotImplemented
	}
	return d.PluginListFn(ctx, filter)
}

func (d *DryRunClient) PluginRemove(ctx context.Context, name string, options moby.PluginRemoveOptions) error {
	if d.PluginRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.PluginRemoveFn(ctx, name, options)
}

func (d *DryRunClient) PluginEnable(ctx context.Context, name string, options moby.PluginEnableOptions) error {
	if d.PluginEnableFn == nil {
		return ErrNotImplemented
	}
	return d.PluginEnableFn(ctx, name, options)
}

func (d *DryRunClient) PluginDisable(ctx context.Context, name string, options moby.PluginDisableOptions) error {
	if d.PluginDisableFn == nil {
		return ErrNotImplemented
	}
	return d.PluginDisableFn(ctx, name, options)
}

func (d *DryRunClient) PluginInstall(ctx context.Context, name string, options moby.PluginInstallOptions) (io.ReadCloser, error) {
	if d.PluginInstallFn == nil {
		return nil, ErrNotImplemented
	}
	return d.PluginInstallFn(ctx, name, options)
}

func (d *DryRunClient) PluginUpgrade(ctx context.Context, name string, options moby.PluginInstallOptions) (io.ReadCloser, error) {
	if d.PluginUpgradeFn == nil {
		return nil, ErrNotImplemented
	}
	return d.PluginUpgradeFn(ctx, name, options)
}

func (d *DryRunClient) PluginPush(ctx context.Context, name string, registryAuth string) (io.ReadCloser, error) {
	if d.PluginPushFn == nil {
		return nil, ErrNotImplemented
	}
	return d.PluginPushFn(ctx, name, registryAuth)
}

func (d *DryRunClient) PluginSet(ctx context.Context, name string, args []string) error {
	if d.PluginSetFn == nil {
		return ErrNotImplemented
	}
	return d.PluginSetFn(ctx, name, args)
}

func (d *DryRunClient) PluginInspectWithRaw(ctx context.Context, name string) (*moby.Plugin, []byte, error) {
	if d.PluginInspectWithRawFn == nil {
		return nil, nil, ErrNotImplemented
	}
	return d.PluginInspectWithRawFn(ctx, name)
}

func (d *DryRunClient) PluginCreate(ctx context.Context, createContext io.Reader, options moby.PluginCreateOptions) error {
	if d.PluginCreateFn == nil {
		return ErrNotImplemented
	}
	return d.PluginCreateFn(ctx, createContext, options)
}

func (d *DryRunClient) ServiceCreate(ctx context.Context, service swarm.ServiceSpec, options moby.ServiceCreateOptions) (moby.ServiceCreateResponse, error) {
	if d.ServiceCreateFn == nil {
		return moby.ServiceCreateResponse{}, ErrNotImplemented
	}
	return d.ServiceCreateFn(ctx, service, options)
}

func (d *DryRunClient) ServiceInspectWithRaw(ctx context.Context, serviceID string, options moby.ServiceInspectOptions) (swarm.Service, []byte, error) {
	if d.ServiceInspectWithRawFn == nil {
		return swarm.Service{}, nil, ErrNotImplemented
	}
	return d.ServiceInspectWithRawFn(ctx, serviceID, options)
}

func (d *DryRunClient) ServiceList(ctx context.Context, options moby.ServiceListOptions) ([]swarm.Service, error) {
	if d.ServiceListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ServiceListFn(ctx, options)
}

func (d *DryRunClient) ServiceRemove(ctx context.Context, serviceID string) error {
	if d.ServiceRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.ServiceRemoveFn(ctx, serviceID)
}

func (d *DryRunClient) ServiceUpdate(ctx context.Context, serviceID string, version swarm.Version, service swarm.ServiceSpec, options moby.ServiceUpdateOptions) (moby.ServiceUpdateResponse, error) {
	if d.ServiceUpdateFn == nil {
		return moby.ServiceUpdateResponse{}, ErrNotImplemented
	}
	return d.ServiceUpdateFn(ctx, serviceID, version, service, options)
}

func (d *DryRunClient) ServiceLogs(ctx context.Context, serviceID string, options moby.ContainerLogsOptions) (io.ReadCloser, error) {
	if d.ServiceLogsFn == nil {
		return nil, ErrNotImplemented
	}
	return d.ServiceLogsFn(ctx, serviceID, options)
}

func (d *DryRunClient) TaskLogs(ctx context.Context, taskID string, options moby.ContainerLogsOptions) (io.ReadCloser, error) {
	if d.TaskLogsFn == nil {
		return nil, ErrNotImplemented
	}
	return d.TaskLogsFn(ctx, taskID, options)
}

func (d *DryRunClient) TaskInspectWithRaw(ctx context.Context, taskID string) (swarm.Task, []byte, error) {
	if d.TaskInspectWithRawFn == nil {
		return swarm.Task{}, nil, ErrNotImplemented
	}
	return d.TaskInspectWithRawFn(ctx, taskID)
}

func (d *DryRunClient) TaskList(ctx context.Context, options moby.TaskListOptions) ([]swarm.Task, error) {
	if d.TaskListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.TaskListFn(ctx, options)
}

func (d *DryRunClient) SwarmInit(ctx context.Context, req swarm.InitRequest) (string, error) {
	if d.SwarmInitFn == nil {
		return "", ErrNotImplemented
	}
	return d.SwarmInitFn(ctx, req)
}

func (d *DryRunClient) SwarmJoin(ctx context.Context, req swarm.JoinRequest) error {
	if d.SwarmJoinFn == nil {
		return ErrNotImplemented
	}
	return d.SwarmJoinFn(ctx, req)
}

func (d *DryRunClient) SwarmGetUnlockKey(ctx context.Context) (moby.SwarmUnlockKeyResponse, error) {
	if d.SwarmGetUnlockKeyFn == nil {
		return moby.SwarmUnlockKeyResponse{}, ErrNotImplemented
	}
	return d.SwarmGetUnlockKeyFn(ctx)
}

func (d *DryRunClient) SwarmUnlock(ctx context.Context, req swarm.UnlockRequest) error {
	if d.SwarmUnlockFn == nil {
		return ErrNotImplemented
	}
	return d.SwarmUnlockFn(ctx, req)
}

func (d *DryRunClient) SwarmLeave(ctx context.Context, force bool) error {
	if d.SwarmLeaveFn == nil {
		return ErrNotImplemented
	}
	return d.SwarmLeaveFn(ctx, force)
}

func (d *DryRunClient) SwarmInspect(ctx context.Context) (swarm.Swarm, error) {
	if d.SwarmInspectFn == nil {
		return swarm.Swarm{}, ErrNotImplemented
	}
	return d.SwarmInspectFn(ctx)
}

func (d *DryRunClient) SwarmUpdate(ctx context.Context, version swarm.Version, swarmSpec swarm.Spec, flags swarm.UpdateFlags) error {
	if d.SwarmUpdateFn == nil {
		return ErrNotImplemented
	}
	return d.SwarmUpdateFn(ctx, version, swarmSpec, flags)
}

func (d *DryRunClient) SecretList(ctx context.Context, options moby.SecretListOptions) ([]swarm.Secret, error) {
	if d.SecretListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.SecretListFn(ctx, options)
}

func (d *DryRunClient) SecretCreate(ctx context.Context, secret swarm.SecretSpec) (moby.SecretCreateResponse, error) {
	if d.SecretCreateFn == nil {
		return moby.SecretCreateResponse{}, ErrNotImplemented
	}
	return d.SecretCreateFn(ctx, secret)
}

func (d *DryRunClient) SecretRemove(ctx context.Context, id string) error {
	if d.SecretRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.SecretRemoveFn(ctx, id)
}

func (d *DryRunClient) SecretInspectWithRaw(ctx context.Context, name string) (swarm.Secret, []byte, error) {
	if d.SecretInspectWithRawFn == nil {
		return swarm.Secret{}, nil, ErrNotImplemented
	}
	return d.SecretInspectWithRawFn(ctx, name)
}

func (d *DryRunClient) SecretUpdate(ctx context.Context, id string, version swarm.Version, secret swarm.SecretSpec) error {
	if d.SecretUpdateFn == nil {
		return ErrNotImplemented
	}
	return d.SecretUpdateFn(ctx, id, version, secret)
}

func (d *DryRunClient) Events(ctx context.Context, options moby.EventsOptions) (<-chan events.Message, <-chan error) {
	if d.EventsFn == nil {
		errC := make(chan error, 1)
		errC <- ErrNotImplemented
		return nil, errC
	}
	return d.EventsFn(ctx, options)
}

func (d *DryRunClient) Info(ctx context.Context) (moby.Info, error) {
	if d.InfoFn == nil {
		return moby.Info{}, ErrNotImplemented
	}
	return d.InfoFn(ctx)
}

func (d *DryRunClient) RegistryLogin(ctx context.Context, auth moby.AuthConfig) (registry.AuthenticateOKBody, error) {
	if d.RegistryLoginFn == nil {
		return registry.AuthenticateOKBody{}, ErrNotImplemented
	}
	return d.RegistryLoginFn(ctx, auth)
}

func (d *DryRunClient) DiskUsage(ctx context.Context, options moby.DiskUsageOptions) (moby.DiskUsage, error) {
	if d.DiskUsageFn == nil {
		return moby.DiskUsage{}, ErrNotImplemented
	}
	return d.DiskUsageFn(ctx, options)
}

func (d *DryRunClient) Ping(ctx context.Context) (moby.Ping, error) {
	if d.PingFn == nil {
		return moby.Ping{}, ErrNotImplemented
	}
	return d.PingFn(ctx)
}

func (d *DryRunClient) VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
	if d.VolumeCreateFn == nil {
		return volume.Volume{}, ErrNotImplemented
	}
	return d.VolumeCreateFn(ctx, options)
}

func (d *DryRunClient) VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error) {
	if d.VolumeInspectFn == nil {
		return volume.Volume{}, ErrNotImplemented
	}
	return d.VolumeInspectFn(ctx, volumeID)
}

func (d *DryRunClient) VolumeInspectWithRaw(ctx context.Context, volumeID string) (volume.Volume, []byte, error) {
	if d.VolumeInspectWithRawFn == nil {
		return volume.Volume{}, nil, ErrNotImplemented
	}
	return d.VolumeInspectWithRawFn(ctx, volumeID)
}

func (d *DryRunClient) VolumeList(ctx context.Context, filter filters.Args) (volume.ListResponse, error) {
	if d.VolumeListFn == nil {
		return volume.ListResponse{}, ErrNotImplemented
	}
	return d.VolumeListFn(ctx, filter)
}

func (d *DryRunClient) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	if d.VolumeRemoveFn == nil {
		return ErrNotImplemented
	}
	return d.VolumeRemoveFn(ctx, volumeID, force)
}

func (d *DryRunClient) VolumesPrune(ctx context.Context, pruneFilter filters.Args) (moby.VolumesPruneReport, error) {
	if d.VolumesPruneFn == nil {
		return moby.VolumesPruneReport{}, ErrNotImplemented
	}
	return d.VolumesPruneFn(ctx, pruneFilter)
}

func (d *DryRunClient) VolumeUpdate(ctx context.Context, volumeID string, version swarm.Version, options volume.UpdateOptions) error {
	if d.VolumeUpdateFn == nil {
		return ErrNotImplemented
	}
	return d.VolumeUpdateFn(ctx, volumeID, version, options)
}

func (d *DryRunClient) ClientVersion() string {
	if d.ClientVersionFn == nil {
		return "undefined"
	}
	return d.ClientVersionFn()
}

func (d *DryRunClient) DaemonHost() string {
	if d.DaemonHostFn == nil {
		return "undefined"
	}
	return d.DaemonHostFn()
}

func (d *DryRunClient) HTTPClient() *http.Client {
	if d.HTTPClientFn == nil {
		return nil
	}
	return d.HTTPClientFn()
}

func (d *DryRunClient) ServerVersion(ctx context.Context) (moby.Version, error) {
	if d.ServerVersionFn == nil {
		return moby.Version{}, ErrNotImplemented
	}
	return d.ServerVersionFn(ctx)
}

func (d *DryRunClient) NegotiateAPIVersion(ctx context.Context) {
	if d.NegotiateAPIVersionFn == nil {
		return
	}
	d.NegotiateAPIVersionFn(ctx)
}

func (d *DryRunClient) NegotiateAPIVersionPing(ping moby.Ping) {
	if d.NegotiateAPIVersionPingFn == nil {
		return
	}
	d.NegotiateAPIVersionPingFn(ping)
}

func (d *DryRunClient) DialHijack(ctx context.Context, url, proto string, meta map[string][]string) (net.Conn, error) {
	if d.DialHijackFn == nil {
		return nil, ErrNotImplemented
	}
	return d.DialHijackFn(ctx, url, proto, meta)
}

func (d *DryRunClient) Dialer() func(context.Context) (net.Conn, error) {
	if d.DialerFn == nil {
		return nil
	}
	return d.DialerFn()
}

func (d *DryRunClient) Close() error {
	if d.CloseFn == nil {
		return ErrNotImplemented
	}
	return d.CloseFn()
}

func (d *DryRunClient) CheckpointCreate(ctx context.Context, container string, options moby.CheckpointCreateOptions) error {
	if d.CheckpointCreateFn == nil {
		return ErrNotImplemented
	}
	return d.CheckpointCreateFn(ctx, container, options)
}

func (d *DryRunClient) CheckpointDelete(ctx context.Context, container string, options moby.CheckpointDeleteOptions) error {
	if d.CheckpointDeleteFn == nil {
		return ErrNotImplemented
	}
	return d.CheckpointDeleteFn(ctx, container, options)
}

func (d *DryRunClient) CheckpointList(ctx context.Context, container string, options moby.CheckpointListOptions) ([]moby.Checkpoint, error) {
	if d.CheckpointListFn == nil {
		return nil, ErrNotImplemented
	}
	return d.CheckpointListFn(ctx, container, options)
}
