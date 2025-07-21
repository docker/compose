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

package dryrun

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
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

var _ client.APIClient = &DryRunClient{}

// DryRunClient implements APIClient by delegating to implementation functions. This allows lazy init and per-method overrides
type DryRunClient struct {
	apiClient  client.APIClient
	containers []containerType.Summary
	execs      sync.Map
	resolver   *imagetools.Resolver
}

type execDetails struct {
	container string
	command   []string
}

type fakeStreamResult struct {
	io.ReadCloser
	client.ImagePushResponse // same interface as [client.ImagePullResponse]
}

func (e fakeStreamResult) Read(p []byte) (int, error) { return e.ReadCloser.Read(p) }
func (e fakeStreamResult) Close() error               { return e.ReadCloser.Close() }

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
		containers: []containerType.Summary{},
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

func (d *DryRunClient) ContainerAttach(ctx context.Context, container string, options client.ContainerAttachOptions) (client.ContainerAttachResult, error) {
	return client.ContainerAttachResult{}, errors.New("interactive run is not supported in dry-run mode")
}

func (d *DryRunClient) ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	d.containers = append(d.containers, containerType.Summary{
		ID:     options.Name,
		Names:  []string{options.Name},
		Labels: options.Config.Labels,
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{},
	})
	return client.ContainerCreateResult{ID: options.Name}, nil
}

func (d *DryRunClient) ContainerInspect(ctx context.Context, container string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	containerJSON, err := d.apiClient.ContainerInspect(ctx, container, options)
	if err != nil {
		id := "dryRunId"
		for _, c := range d.containers {
			if c.ID == container {
				id = container
			}
		}
		return client.ContainerInspectResult{
			Container: containerType.InspectResponse{
				ID:   id,
				Name: container,
				State: &containerType.State{
					Status: containerType.StateRunning, // needed for --wait option
					Health: &containerType.Health{
						Status: containerType.Healthy, // needed for healthcheck control
					},
				},
				Mounts:          nil,
				Config:          &containerType.Config{},
				NetworkSettings: &containerType.NetworkSettings{},
			},
		}, nil
	}
	return containerJSON, err
}

func (d *DryRunClient) ContainerKill(ctx context.Context, container string, options client.ContainerKillOptions) (client.ContainerKillResult, error) {
	return client.ContainerKillResult{}, nil
}

func (d *DryRunClient) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	caller := getCallingFunction()
	switch caller {
	case "start":
		return client.ContainerListResult{
			Items: d.containers,
		}, nil
	case "getContainers":
		if len(d.containers) == 0 {
			res, err := d.apiClient.ContainerList(ctx, options)
			if err == nil {
				d.containers = res.Items
			}
			return client.ContainerListResult{
				Items: d.containers,
			}, err
		}
	}
	return d.apiClient.ContainerList(ctx, options)
}

func (d *DryRunClient) ContainerPause(ctx context.Context, container string, options client.ContainerPauseOptions) (client.ContainerPauseResult, error) {
	return client.ContainerPauseResult{}, nil
}

func (d *DryRunClient) ContainerRemove(ctx context.Context, container string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (d *DryRunClient) ContainerRename(ctx context.Context, container string, options client.ContainerRenameOptions) (client.ContainerRenameResult, error) {
	return client.ContainerRenameResult{}, nil
}

func (d *DryRunClient) ContainerRestart(ctx context.Context, container string, options client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
	return client.ContainerRestartResult{}, nil
}

func (d *DryRunClient) ContainerStart(ctx context.Context, container string, options client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, nil
}

func (d *DryRunClient) ContainerStop(ctx context.Context, container string, options client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}

func (d *DryRunClient) ContainerUnpause(ctx context.Context, container string, options client.ContainerUnpauseOptions) (client.ContainerUnpauseResult, error) {
	return client.ContainerUnpauseResult{}, nil
}

func (d *DryRunClient) CopyFromContainer(ctx context.Context, container string, options client.CopyFromContainerOptions) (client.CopyFromContainerResult, error) {
	if _, err := d.ContainerStatPath(ctx, container, client.ContainerStatPathOptions{Path: options.SourcePath}); err != nil {
		return client.CopyFromContainerResult{}, fmt.Errorf("could not find the file %s in container %s", options.SourcePath, container)
	}
	return client.CopyFromContainerResult{}, nil
}

func (d *DryRunClient) CopyToContainer(ctx context.Context, container string, options client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (d *DryRunClient) ImageBuild(ctx context.Context, reader io.Reader, options client.ImageBuildOptions) (client.ImageBuildResult, error) {
	return client.ImageBuildResult{
		Body: io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

func (d *DryRunClient) ImageInspect(ctx context.Context, imageName string, options ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	caller := getCallingFunction()
	switch caller {
	case "pullServiceImage", "buildContainerVolumes":
		return client.ImageInspectResult{
			InspectResponse: image.InspectResponse{ID: "dryRunId"},
		}, nil
	default:
		return d.apiClient.ImageInspect(ctx, imageName, options...)
	}
}

func (d *DryRunClient) ImagePull(ctx context.Context, ref string, options client.ImagePullOptions) (client.ImagePullResponse, error) {
	if _, _, err := d.resolver.Resolve(ctx, ref); err != nil {
		return nil, err
	}
	return fakeStreamResult{ReadCloser: http.NoBody}, nil
}

func (d *DryRunClient) ImagePush(ctx context.Context, ref string, options client.ImagePushOptions) (client.ImagePushResponse, error) {
	if _, _, err := d.resolver.Resolve(ctx, ref); err != nil {
		return nil, err
	}
	jsonMessage, err := json.Marshal(&jsonstream.Message{
		Status: "Pushed",
		Progress: &jsonstream.Progress{
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
	return fakeStreamResult{ReadCloser: io.NopCloser(bytes.NewReader(jsonMessage))}, nil
}

func (d *DryRunClient) ImageRemove(ctx context.Context, imageName string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (d *DryRunClient) NetworkConnect(ctx context.Context, networkName string, options client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
	return client.NetworkConnectResult{}, nil
}

func (d *DryRunClient) NetworkCreate(ctx context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
	return client.NetworkCreateResult{
		ID: name,
	}, nil
}

func (d *DryRunClient) NetworkDisconnect(ctx context.Context, networkName string, options client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error) {
	return client.NetworkDisconnectResult{}, nil
}

func (d *DryRunClient) NetworkRemove(ctx context.Context, networkName string, options client.NetworkRemoveOptions) (client.NetworkRemoveResult, error) {
	return client.NetworkRemoveResult{}, nil
}

func (d *DryRunClient) VolumeCreate(ctx context.Context, options client.VolumeCreateOptions) (client.VolumeCreateResult, error) {
	return client.VolumeCreateResult{
		Volume: volume.Volume{
			ClusterVolume: nil,
			Driver:        options.Driver,
			Labels:        options.Labels,
			Name:          options.Name,
			Options:       options.DriverOpts,
		},
	}, nil
}

func (d *DryRunClient) VolumeRemove(ctx context.Context, volumeID string, options client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (d *DryRunClient) ExecCreate(ctx context.Context, container string, config client.ExecCreateOptions) (client.ExecCreateResult, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	id := fmt.Sprintf("%x", b)
	d.execs.Store(id, execDetails{
		container: container,
		command:   config.Cmd,
	})
	return client.ExecCreateResult{
		ID: id,
	}, nil
}

func (d *DryRunClient) ExecStart(ctx context.Context, execID string, config client.ExecStartOptions) (client.ExecStartResult, error) {
	_, ok := d.execs.LoadAndDelete(execID)
	if !ok {
		return client.ExecStartResult{}, fmt.Errorf("invalid exec ID %q", execID)
	}
	return client.ExecStartResult{}, nil
}

// Functions delegated to original APIClient (not used by Compose or not modifying the Compose stack)

func (d *DryRunClient) ConfigList(ctx context.Context, options client.ConfigListOptions) (client.ConfigListResult, error) {
	return d.apiClient.ConfigList(ctx, options)
}

func (d *DryRunClient) ConfigInspect(ctx context.Context, name string, options client.ConfigInspectOptions) (client.ConfigInspectResult, error) {
	return d.apiClient.ConfigInspect(ctx, name, options)
}

func (d *DryRunClient) ConfigCreate(ctx context.Context, options client.ConfigCreateOptions) (client.ConfigCreateResult, error) {
	return d.apiClient.ConfigCreate(ctx, options)
}

func (d *DryRunClient) ConfigRemove(ctx context.Context, id string, options client.ConfigRemoveOptions) (client.ConfigRemoveResult, error) {
	return d.apiClient.ConfigRemove(ctx, id, options)
}

func (d *DryRunClient) ConfigUpdate(ctx context.Context, id string, options client.ConfigUpdateOptions) (client.ConfigUpdateResult, error) {
	return d.apiClient.ConfigUpdate(ctx, id, options)
}

func (d *DryRunClient) ContainerCommit(ctx context.Context, container string, options client.ContainerCommitOptions) (client.ContainerCommitResult, error) {
	return d.apiClient.ContainerCommit(ctx, container, options)
}

func (d *DryRunClient) ContainerDiff(ctx context.Context, container string, options client.ContainerDiffOptions) (client.ContainerDiffResult, error) {
	return d.apiClient.ContainerDiff(ctx, container, options)
}

func (d *DryRunClient) ExecAttach(ctx context.Context, execID string, config client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, errors.New("interactive exec is not supported in dry-run mode")
}

func (d *DryRunClient) ExecInspect(ctx context.Context, execID string, options client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return d.apiClient.ExecInspect(ctx, execID, options)
}

func (d *DryRunClient) ExecResize(ctx context.Context, execID string, options client.ExecResizeOptions) (client.ExecResizeResult, error) {
	return d.apiClient.ExecResize(ctx, execID, options)
}

func (d *DryRunClient) ContainerExport(ctx context.Context, container string, options client.ContainerExportOptions) (client.ContainerExportResult, error) {
	return d.apiClient.ContainerExport(ctx, container, options)
}

func (d *DryRunClient) ContainerLogs(ctx context.Context, container string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return d.apiClient.ContainerLogs(ctx, container, options)
}

func (d *DryRunClient) ContainerResize(ctx context.Context, container string, options client.ContainerResizeOptions) (client.ContainerResizeResult, error) {
	return d.apiClient.ContainerResize(ctx, container, options)
}

func (d *DryRunClient) ContainerStatPath(ctx context.Context, container string, options client.ContainerStatPathOptions) (client.ContainerStatPathResult, error) {
	return d.apiClient.ContainerStatPath(ctx, container, options)
}

func (d *DryRunClient) ContainerStats(ctx context.Context, container string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	return d.apiClient.ContainerStats(ctx, container, options)
}

func (d *DryRunClient) ContainerTop(ctx context.Context, container string, options client.ContainerTopOptions) (client.ContainerTopResult, error) {
	return d.apiClient.ContainerTop(ctx, container, options)
}

func (d *DryRunClient) ContainerUpdate(ctx context.Context, container string, options client.ContainerUpdateOptions) (client.ContainerUpdateResult, error) {
	return d.apiClient.ContainerUpdate(ctx, container, options)
}

func (d *DryRunClient) ContainerWait(ctx context.Context, container string, options client.ContainerWaitOptions) client.ContainerWaitResult {
	return d.apiClient.ContainerWait(ctx, container, options)
}

func (d *DryRunClient) ContainerPrune(ctx context.Context, options client.ContainerPruneOptions) (client.ContainerPruneResult, error) {
	return d.apiClient.ContainerPrune(ctx, options)
}

func (d *DryRunClient) DistributionInspect(ctx context.Context, imageName string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
	return d.apiClient.DistributionInspect(ctx, imageName, options)
}

func (d *DryRunClient) BuildCachePrune(ctx context.Context, opts client.BuildCachePruneOptions) (client.BuildCachePruneResult, error) {
	return d.apiClient.BuildCachePrune(ctx, opts)
}

func (d *DryRunClient) BuildCancel(ctx context.Context, id string, opts client.BuildCancelOptions) (client.BuildCancelResult, error) {
	return d.apiClient.BuildCancel(ctx, id, opts)
}

func (d *DryRunClient) ImageHistory(ctx context.Context, imageName string, options ...client.ImageHistoryOption) (client.ImageHistoryResult, error) {
	return d.apiClient.ImageHistory(ctx, imageName, options...)
}

func (d *DryRunClient) ImageImport(ctx context.Context, source client.ImageImportSource, ref string, options client.ImageImportOptions) (client.ImageImportResult, error) {
	return d.apiClient.ImageImport(ctx, source, ref, options)
}

func (d *DryRunClient) ImageList(ctx context.Context, options client.ImageListOptions) (client.ImageListResult, error) {
	return d.apiClient.ImageList(ctx, options)
}

func (d *DryRunClient) ImageLoad(ctx context.Context, input io.Reader, options ...client.ImageLoadOption) (client.ImageLoadResult, error) {
	return d.apiClient.ImageLoad(ctx, input, options...)
}

func (d *DryRunClient) ImageSearch(ctx context.Context, term string, options client.ImageSearchOptions) (client.ImageSearchResult, error) {
	return d.apiClient.ImageSearch(ctx, term, options)
}

func (d *DryRunClient) ImageSave(ctx context.Context, images []string, options ...client.ImageSaveOption) (client.ImageSaveResult, error) {
	return d.apiClient.ImageSave(ctx, images, options...)
}

func (d *DryRunClient) ImageTag(ctx context.Context, options client.ImageTagOptions) (client.ImageTagResult, error) {
	return d.apiClient.ImageTag(ctx, options)
}

func (d *DryRunClient) ImagePrune(ctx context.Context, options client.ImagePruneOptions) (client.ImagePruneResult, error) {
	return d.apiClient.ImagePrune(ctx, options)
}

func (d *DryRunClient) NodeInspect(ctx context.Context, nodeID string, options client.NodeInspectOptions) (client.NodeInspectResult, error) {
	return d.apiClient.NodeInspect(ctx, nodeID, options)
}

func (d *DryRunClient) NodeList(ctx context.Context, options client.NodeListOptions) (client.NodeListResult, error) {
	return d.apiClient.NodeList(ctx, options)
}

func (d *DryRunClient) NodeRemove(ctx context.Context, nodeID string, options client.NodeRemoveOptions) (client.NodeRemoveResult, error) {
	return d.apiClient.NodeRemove(ctx, nodeID, options)
}

func (d *DryRunClient) NodeUpdate(ctx context.Context, nodeID string, options client.NodeUpdateOptions) (client.NodeUpdateResult, error) {
	return d.apiClient.NodeUpdate(ctx, nodeID, options)
}

func (d *DryRunClient) NetworkInspect(ctx context.Context, networkName string, options client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	return d.apiClient.NetworkInspect(ctx, networkName, options)
}

func (d *DryRunClient) NetworkList(ctx context.Context, options client.NetworkListOptions) (client.NetworkListResult, error) {
	return d.apiClient.NetworkList(ctx, options)
}

func (d *DryRunClient) NetworkPrune(ctx context.Context, options client.NetworkPruneOptions) (client.NetworkPruneResult, error) {
	return d.apiClient.NetworkPrune(ctx, options)
}

func (d *DryRunClient) PluginList(ctx context.Context, options client.PluginListOptions) (client.PluginListResult, error) {
	return d.apiClient.PluginList(ctx, options)
}

func (d *DryRunClient) PluginRemove(ctx context.Context, name string, options client.PluginRemoveOptions) (client.PluginRemoveResult, error) {
	return d.apiClient.PluginRemove(ctx, name, options)
}

func (d *DryRunClient) PluginEnable(ctx context.Context, name string, options client.PluginEnableOptions) (client.PluginEnableResult, error) {
	return d.apiClient.PluginEnable(ctx, name, options)
}

func (d *DryRunClient) PluginDisable(ctx context.Context, name string, options client.PluginDisableOptions) (client.PluginDisableResult, error) {
	return d.apiClient.PluginDisable(ctx, name, options)
}

func (d *DryRunClient) PluginInstall(ctx context.Context, name string, options client.PluginInstallOptions) (client.PluginInstallResult, error) {
	return d.apiClient.PluginInstall(ctx, name, options)
}

func (d *DryRunClient) PluginUpgrade(ctx context.Context, name string, options client.PluginUpgradeOptions) (client.PluginUpgradeResult, error) {
	return d.apiClient.PluginUpgrade(ctx, name, options)
}

func (d *DryRunClient) PluginPush(ctx context.Context, name string, options client.PluginPushOptions) (client.PluginPushResult, error) {
	return d.apiClient.PluginPush(ctx, name, options)
}

func (d *DryRunClient) PluginSet(ctx context.Context, name string, options client.PluginSetOptions) (client.PluginSetResult, error) {
	return d.apiClient.PluginSet(ctx, name, options)
}

func (d *DryRunClient) PluginInspect(ctx context.Context, name string, options client.PluginInspectOptions) (client.PluginInspectResult, error) {
	return d.apiClient.PluginInspect(ctx, name, options)
}

func (d *DryRunClient) PluginCreate(ctx context.Context, createContext io.Reader, options client.PluginCreateOptions) (client.PluginCreateResult, error) {
	return d.apiClient.PluginCreate(ctx, createContext, options)
}

func (d *DryRunClient) ServiceCreate(ctx context.Context, options client.ServiceCreateOptions) (client.ServiceCreateResult, error) {
	return d.apiClient.ServiceCreate(ctx, options)
}

func (d *DryRunClient) ServiceInspect(ctx context.Context, serviceID string, options client.ServiceInspectOptions) (client.ServiceInspectResult, error) {
	return d.apiClient.ServiceInspect(ctx, serviceID, options)
}

func (d *DryRunClient) ServiceList(ctx context.Context, options client.ServiceListOptions) (client.ServiceListResult, error) {
	return d.apiClient.ServiceList(ctx, options)
}

func (d *DryRunClient) ServiceRemove(ctx context.Context, serviceID string, options client.ServiceRemoveOptions) (client.ServiceRemoveResult, error) {
	return d.apiClient.ServiceRemove(ctx, serviceID, options)
}

func (d *DryRunClient) ServiceUpdate(ctx context.Context, serviceID string, options client.ServiceUpdateOptions) (client.ServiceUpdateResult, error) {
	return d.apiClient.ServiceUpdate(ctx, serviceID, options)
}

func (d *DryRunClient) ServiceLogs(ctx context.Context, serviceID string, options client.ServiceLogsOptions) (client.ServiceLogsResult, error) {
	return d.apiClient.ServiceLogs(ctx, serviceID, options)
}

func (d *DryRunClient) TaskLogs(ctx context.Context, taskID string, options client.TaskLogsOptions) (client.TaskLogsResult, error) {
	return d.apiClient.TaskLogs(ctx, taskID, options)
}

func (d *DryRunClient) TaskInspect(ctx context.Context, taskID string, options client.TaskInspectOptions) (client.TaskInspectResult, error) {
	return d.apiClient.TaskInspect(ctx, taskID, options)
}

func (d *DryRunClient) TaskList(ctx context.Context, options client.TaskListOptions) (client.TaskListResult, error) {
	return d.apiClient.TaskList(ctx, options)
}

func (d *DryRunClient) SwarmInit(ctx context.Context, options client.SwarmInitOptions) (client.SwarmInitResult, error) {
	return d.apiClient.SwarmInit(ctx, options)
}

func (d *DryRunClient) SwarmJoin(ctx context.Context, options client.SwarmJoinOptions) (client.SwarmJoinResult, error) {
	return d.apiClient.SwarmJoin(ctx, options)
}

func (d *DryRunClient) SwarmGetUnlockKey(ctx context.Context) (client.SwarmGetUnlockKeyResult, error) {
	return d.apiClient.SwarmGetUnlockKey(ctx)
}

func (d *DryRunClient) SwarmUnlock(ctx context.Context, options client.SwarmUnlockOptions) (client.SwarmUnlockResult, error) {
	return d.apiClient.SwarmUnlock(ctx, options)
}

func (d *DryRunClient) SwarmLeave(ctx context.Context, options client.SwarmLeaveOptions) (client.SwarmLeaveResult, error) {
	return d.apiClient.SwarmLeave(ctx, options)
}

func (d *DryRunClient) SwarmInspect(ctx context.Context, options client.SwarmInspectOptions) (client.SwarmInspectResult, error) {
	return d.apiClient.SwarmInspect(ctx, options)
}

func (d *DryRunClient) SwarmUpdate(ctx context.Context, options client.SwarmUpdateOptions) (client.SwarmUpdateResult, error) {
	return d.apiClient.SwarmUpdate(ctx, options)
}

func (d *DryRunClient) SecretList(ctx context.Context, options client.SecretListOptions) (client.SecretListResult, error) {
	return d.apiClient.SecretList(ctx, options)
}

func (d *DryRunClient) SecretCreate(ctx context.Context, options client.SecretCreateOptions) (client.SecretCreateResult, error) {
	return d.apiClient.SecretCreate(ctx, options)
}

func (d *DryRunClient) SecretRemove(ctx context.Context, id string, options client.SecretRemoveOptions) (client.SecretRemoveResult, error) {
	return d.apiClient.SecretRemove(ctx, id, options)
}

func (d *DryRunClient) SecretInspect(ctx context.Context, name string, options client.SecretInspectOptions) (client.SecretInspectResult, error) {
	return d.apiClient.SecretInspect(ctx, name, options)
}

func (d *DryRunClient) SecretUpdate(ctx context.Context, id string, options client.SecretUpdateOptions) (client.SecretUpdateResult, error) {
	return d.apiClient.SecretUpdate(ctx, id, options)
}

func (d *DryRunClient) Events(ctx context.Context, options client.EventsListOptions) client.EventsResult {
	return d.apiClient.Events(ctx, options)
}

func (d *DryRunClient) Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error) {
	return d.apiClient.Info(ctx, options)
}

func (d *DryRunClient) RegistryLogin(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error) {
	return d.apiClient.RegistryLogin(ctx, options)
}

func (d *DryRunClient) DiskUsage(ctx context.Context, options client.DiskUsageOptions) (client.DiskUsageResult, error) {
	return d.apiClient.DiskUsage(ctx, options)
}

func (d *DryRunClient) Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error) {
	return d.apiClient.Ping(ctx, options)
}

func (d *DryRunClient) VolumeInspect(ctx context.Context, volumeID string, options client.VolumeInspectOptions) (client.VolumeInspectResult, error) {
	return d.apiClient.VolumeInspect(ctx, volumeID, options)
}

func (d *DryRunClient) VolumeList(ctx context.Context, opts client.VolumeListOptions) (client.VolumeListResult, error) {
	return d.apiClient.VolumeList(ctx, opts)
}

func (d *DryRunClient) VolumePrune(ctx context.Context, options client.VolumePruneOptions) (client.VolumePruneResult, error) {
	return d.apiClient.VolumePrune(ctx, options)
}

func (d *DryRunClient) VolumeUpdate(ctx context.Context, volumeID string, options client.VolumeUpdateOptions) (client.VolumeUpdateResult, error) {
	return d.apiClient.VolumeUpdate(ctx, volumeID, options)
}

func (d *DryRunClient) ClientVersion() string {
	return d.apiClient.ClientVersion()
}

func (d *DryRunClient) DaemonHost() string {
	return d.apiClient.DaemonHost()
}

func (d *DryRunClient) ServerVersion(ctx context.Context, options client.ServerVersionOptions) (client.ServerVersionResult, error) {
	return d.apiClient.ServerVersion(ctx, options)
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

func (d *DryRunClient) CheckpointCreate(ctx context.Context, container string, options client.CheckpointCreateOptions) (client.CheckpointCreateResult, error) {
	return d.apiClient.CheckpointCreate(ctx, container, options)
}

func (d *DryRunClient) CheckpointRemove(ctx context.Context, container string, options client.CheckpointRemoveOptions) (client.CheckpointRemoveResult, error) {
	return d.apiClient.CheckpointRemove(ctx, container, options)
}

func (d *DryRunClient) CheckpointList(ctx context.Context, container string, options client.CheckpointListOptions) (client.CheckpointListResult, error) {
	return d.apiClient.CheckpointList(ctx, container, options)
}
