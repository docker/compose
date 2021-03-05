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

package aci

import (
	"context"
	"fmt"
	"net/http"

	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/utils/formatter"
)

type aciComposeService struct {
	ctx          store.AciContext
	storageLogin login.StorageLoginImpl
}

func newComposeService(ctx store.AciContext) aciComposeService {
	return aciComposeService{
		ctx:          ctx,
		storageLogin: login.StorageLoginImpl{AciContext: ctx},
	}
}

func (cs *aciComposeService) Build(ctx context.Context, project *types.Project, options compose.BuildOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Push(ctx context.Context, project *types.Project, options compose.PushOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Pull(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Create(ctx context.Context, project *types.Project, opts compose.CreateOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Stop(ctx context.Context, project *types.Project, options compose.StopOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Pause(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) UnPause(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Up(ctx context.Context, project *types.Project, options compose.UpOptions) error {
	logrus.Debugf("Up on project with name %q", project.Name)

	if err := autocreateFileshares(ctx, project); err != nil {
		return err
	}

	groupDefinition, err := convert.ToContainerGroup(ctx, cs.ctx, *project, cs.storageLogin)
	if err != nil {
		return err
	}

	addTag(&groupDefinition, composeContainerTag)
	return createOrUpdateACIContainers(ctx, cs.ctx, groupDefinition)
}

func (cs aciComposeService) warnKeepVolumeOnDown(ctx context.Context, projectName string) error {
	cgClient, err := login.NewContainerGroupsClient(cs.ctx.SubscriptionID)
	if err != nil {
		return err
	}
	cg, err := cgClient.Get(ctx, cs.ctx.ResourceGroup, projectName)
	if err != nil {
		return err
	}
	if cg.Volumes == nil {
		return nil
	}
	for _, v := range *cg.Volumes {
		if v.AzureFile == nil || v.AzureFile.StorageAccountName == nil || v.AzureFile.ShareName == nil {
			continue
		}
		fmt.Printf("WARNING: fileshare \"%s/%s\" will NOT be deleted. Use 'docker volume rm' if you want to delete this volume\n",
			*v.AzureFile.StorageAccountName, *v.AzureFile.ShareName)
	}
	return nil
}

func (cs *aciComposeService) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	logrus.Debugf("Down on projectName with name %q", projectName)

	if err := cs.warnKeepVolumeOnDown(ctx, projectName); err != nil {
		return err
	}

	cg, err := deleteACIContainerGroup(ctx, cs.ctx, projectName)
	if err != nil {
		return err
	}
	if cg.IsHTTPStatus(http.StatusNoContent) {
		return errdefs.ErrNotFound
	}

	return err
}

func (cs *aciComposeService) Ps(ctx context.Context, projectName string, options compose.PsOptions) ([]compose.ContainerSummary, error) {
	groupsClient, err := login.NewContainerGroupsClient(cs.ctx.SubscriptionID)
	if err != nil {
		return nil, err
	}

	group, err := groupsClient.Get(ctx, cs.ctx.ResourceGroup, projectName)
	if err != nil {
		return nil, err
	}

	if group.Containers == nil || len(*group.Containers) == 0 {
		return nil, fmt.Errorf("no containers found in ACI container group %s", projectName)
	}

	res := []compose.ContainerSummary{}
	for _, container := range *group.Containers {
		if isContainerVisible(container, group, false) {
			continue
		}
		var publishers []compose.PortPublisher
		urls := formatter.PortsToStrings(convert.ToPorts(group.IPAddress, *container.Ports), convert.FQDN(group, cs.ctx.Location))
		for i, p := range *container.Ports {
			publishers = append(publishers, compose.PortPublisher{
				URL:           urls[i],
				TargetPort:    int(*p.Port),
				PublishedPort: int(*p.Port),
				Protocol:      string(p.Protocol),
			})
		}
		id := getContainerID(group, container)
		res = append(res, compose.ContainerSummary{
			ID:         id,
			Name:       id,
			Project:    projectName,
			Service:    *container.Name,
			State:      convert.GetStatus(container, group),
			Publishers: publishers,
		})
	}
	return res, nil
}

func (cs *aciComposeService) List(ctx context.Context, opts compose.ListOptions) ([]compose.Stack, error) {
	containerGroups, err := getACIContainerGroups(ctx, cs.ctx.SubscriptionID, cs.ctx.ResourceGroup)
	if err != nil {
		return nil, err
	}

	var stacks []compose.Stack
	for _, group := range containerGroups {
		if _, found := group.Tags[composeContainerTag]; !found {
			continue
		}
		state := compose.RUNNING
		for _, container := range *group.ContainerGroupProperties.Containers {
			containerState := convert.GetStatus(container, group)
			if containerState != compose.RUNNING {
				state = containerState
				break
			}
		}
		stacks = append(stacks, compose.Stack{
			ID:     *group.ID,
			Name:   *group.Name,
			Status: state,
		})
	}
	return stacks, nil
}

func (cs *aciComposeService) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Convert(ctx context.Context, project *types.Project, options compose.ConvertOptions) ([]byte, error) {
	return nil, errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Kill(ctx context.Context, project *types.Project, options compose.KillOptions) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) (int, error) {
	return 0, errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Remove(ctx context.Context, project *types.Project, options compose.RemoveOptions) ([]string, error) {
	return nil, errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Exec(ctx context.Context, project *types.Project, opts compose.RunOptions) error {
	return errdefs.ErrNotImplemented
}
