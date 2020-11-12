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
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
)

type aciContainerService struct {
	ctx          store.AciContext
	storageLogin login.StorageLoginImpl
}

func newContainerService(ctx store.AciContext) aciContainerService {
	return aciContainerService{
		ctx:          ctx,
		storageLogin: login.StorageLoginImpl{AciContext: ctx},
	}
}

func (cs *aciContainerService) List(ctx context.Context, all bool) ([]containers.Container, error) {
	containerGroups, err := getACIContainerGroups(ctx, cs.ctx.SubscriptionID, cs.ctx.ResourceGroup)
	if err != nil {
		return nil, err
	}
	res := []containers.Container{}
	for _, group := range containerGroups {
		if group.Containers == nil || len(*group.Containers) == 0 {
			return nil, fmt.Errorf("no containers found in ACI container group %s", *group.Name)
		}

		for _, container := range *group.Containers {
			if isContainerVisible(container, group, all) {
				continue
			}
			c := convert.ContainerGroupToContainer(getContainerID(group, container), group, container, cs.ctx.Location)
			res = append(res, c)
		}
	}
	return res, nil
}

func (cs *aciContainerService) Run(ctx context.Context, r containers.ContainerConfig) error {
	if strings.Contains(r.ID, composeContainerSeparator) {
		return fmt.Errorf("invalid container name. ACI container name cannot include %q", composeContainerSeparator)
	}

	project, err := convert.ContainerToComposeProject(r)
	if err != nil {
		return err
	}

	logrus.Debugf("Running container %q with name %q", r.Image, r.ID)
	groupDefinition, err := convert.ToContainerGroup(ctx, cs.ctx, project, cs.storageLogin)
	if err != nil {
		return err
	}
	addTag(&groupDefinition, singleContainerTag)

	return createACIContainers(ctx, cs.ctx, groupDefinition)
}

func (cs *aciContainerService) Start(ctx context.Context, containerID string) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot start specified service %q from compose application %q, you can update and restart the entire compose app with docker compose up --project-name %s"
		return fmt.Errorf(msg, containerName, groupName, groupName)
	}

	containerGroupsClient, err := login.NewContainerGroupsClient(cs.ctx.SubscriptionID)
	if err != nil {
		return err
	}

	future, err := containerGroupsClient.Start(ctx, cs.ctx.ResourceGroup, containerName)
	if err != nil {
		var aerr autorest.DetailedError
		if ok := errors.As(err, &aerr); ok {
			if aerr.StatusCode == http.StatusNotFound {
				return errdefs.ErrNotFound
			}
		}
		return err
	}

	return future.WaitForCompletionRef(ctx, containerGroupsClient.Client)
}

func (cs *aciContainerService) Stop(ctx context.Context, containerID string, timeout *uint32) error {
	if timeout != nil && *timeout != uint32(0) {
		return fmt.Errorf("the ACI integration does not support setting a timeout to stop a container before killing it")
	}
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot stop service %q from compose application %q, you can stop the entire compose app with docker stop %s"
		return fmt.Errorf(msg, containerName, groupName, groupName)
	}
	return stopACIContainerGroup(ctx, cs.ctx, groupName)
}

func (cs *aciContainerService) Kill(ctx context.Context, containerID string, _ string) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot kill service %q from compose application %q, you can kill the entire compose app with docker kill %s"
		return fmt.Errorf(msg, containerName, groupName, groupName)
	}
	return stopACIContainerGroup(ctx, cs.ctx, groupName) // As ACI doesn't have a kill command, we are using the stop implementation instead
}

func (cs *aciContainerService) Exec(ctx context.Context, name string, request containers.ExecRequest) error {
	err := verifyExecCommand(request.Command)
	if err != nil {
		return err
	}
	groupName, containerAciName := getGroupAndContainerName(name)
	containerExecResponse, err := execACIContainer(ctx, cs.ctx, request.Command, groupName, containerAciName)
	if err != nil {
		return err
	}

	return exec(
		context.Background(),
		*containerExecResponse.WebSocketURI,
		*containerExecResponse.Password,
		request,
	)
}

func verifyExecCommand(command string) error {
	tokens := strings.Split(command, " ")
	if len(tokens) > 1 {
		return errors.New("ACI exec command does not accept arguments to the command. " +
			"Only the binary should be specified")
	}
	return nil
}

func (cs *aciContainerService) Logs(ctx context.Context, containerName string, req containers.LogsRequest) error {
	groupName, containerAciName := getGroupAndContainerName(containerName)
	var tail *int32

	if req.Follow {
		return streamLogs(ctx, cs.ctx, groupName, containerAciName, req)
	}

	if req.Tail != "all" {
		reqTail, err := strconv.Atoi(req.Tail)
		if err != nil {
			return err
		}
		i32 := int32(reqTail)
		tail = &i32
	}

	logs, err := getACIContainerLogs(ctx, cs.ctx, groupName, containerAciName, tail)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(req.Writer, logs)
	return err
}

func (cs *aciContainerService) Delete(ctx context.Context, containerID string, request containers.DeleteRequest) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot delete service %q from compose application %q, you can delete the entire compose app with docker compose down --project-name %s"
		return fmt.Errorf(msg, containerName, groupName, groupName)
	}

	if !request.Force {
		containerGroupsClient, err := login.NewContainerGroupsClient(cs.ctx.SubscriptionID)
		if err != nil {
			return err
		}

		cg, err := containerGroupsClient.Get(ctx, cs.ctx.ResourceGroup, groupName)
		if err != nil {
			if cg.StatusCode == http.StatusNotFound {
				return errdefs.ErrNotFound
			}
			return err
		}

		for _, container := range *cg.Containers {
			status := convert.GetStatus(container, cg)

			if status == convert.StatusRunning {
				return errdefs.ErrForbidden
			}
		}
	}

	cg, err := deleteACIContainerGroup(ctx, cs.ctx, groupName)
	// Delete returns `StatusNoContent` if the group is not found
	if cg.IsHTTPStatus(http.StatusNoContent) {
		return errdefs.ErrNotFound
	}
	if err != nil {
		return err
	}

	return err
}

func (cs *aciContainerService) Inspect(ctx context.Context, containerID string) (containers.Container, error) {
	groupName, containerName := getGroupAndContainerName(containerID)
	if containerID == "" {
		return containers.Container{}, errors.New("cannot inspect empty container ID")
	}

	cg, err := getACIContainerGroup(ctx, cs.ctx, groupName)
	if err != nil {
		return containers.Container{}, err
	}
	if cg.IsHTTPStatus(http.StatusNoContent) || cg.ContainerGroupProperties == nil || cg.ContainerGroupProperties.Containers == nil {
		return containers.Container{}, errdefs.ErrNotFound
	}

	var cc containerinstance.Container
	var found = false
	for _, c := range *cg.Containers {
		if to.String(c.Name) == containerName {
			cc = c
			found = true
			break
		}
	}
	if !found {
		return containers.Container{}, errdefs.ErrNotFound
	}

	return convert.ContainerGroupToContainer(containerID, cg, cc, cs.ctx.Location), nil
}
