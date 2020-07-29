/*
   Copyright 2020 Docker, Inc.

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
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/docker/api/aci/convert"
	"github.com/docker/api/aci/login"
	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/cloud"
	"github.com/docker/api/context/store"
	"github.com/docker/api/errdefs"
)

const (
	backendType               = store.AciContextType
	singleContainerTag        = "docker-single-container"
	composeContainerTag       = "docker-compose-application"
	composeContainerSeparator = "_"
	statusUnknown             = "Unknown"
)

// ErrNoSuchContainer is returned when the mentioned container does not exist
var ErrNoSuchContainer = errors.New("no such container")

// ContextParams options for creating ACI context
type ContextParams struct {
	Description    string
	Location       string
	SubscriptionID string
	ResourceGroup  string
}

// LoginParams azure login options
type LoginParams struct {
	TenantID string
}

func init() {
	backend.Register(backendType, backendType, service, getCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	contextStore := store.ContextStore(ctx)
	currentContext := apicontext.CurrentContext(ctx)
	var aciContext store.AciContext

	if err := contextStore.GetEndpoint(currentContext, &aciContext); err != nil {
		return nil, err
	}

	return getAciAPIService(aciContext), nil
}

func getCloudService() (cloud.Service, error) {
	service, err := login.NewAzureLoginService()
	if err != nil {
		return nil, err
	}
	return &aciCloudService{
		loginService: service,
	}, nil
}

func getAciAPIService(aciCtx store.AciContext) *aciAPIService {
	return &aciAPIService{
		aciContainerService: &aciContainerService{
			ctx: aciCtx,
		},
		aciComposeService: &aciComposeService{
			ctx: aciCtx,
		},
	}
}

type aciAPIService struct {
	*aciContainerService
	*aciComposeService
}

func (a *aciAPIService) ContainerService() containers.Service {
	return a.aciContainerService
}

func (a *aciAPIService) ComposeService() compose.Service {
	return a.aciComposeService
}

type aciContainerService struct {
	ctx store.AciContext
}

func (cs *aciContainerService) List(ctx context.Context, _ bool) ([]containers.Container, error) {
	groupsClient, err := getContainerGroupsClient(cs.ctx.SubscriptionID)
	if err != nil {
		return nil, err
	}
	var containerGroups []containerinstance.ContainerGroup
	result, err := groupsClient.ListByResourceGroup(ctx, cs.ctx.ResourceGroup)
	if err != nil {
		return []containers.Container{}, err
	}

	for result.NotDone() {
		containerGroups = append(containerGroups, result.Values()...)
		if err := result.NextWithContext(ctx); err != nil {
			return []containers.Container{}, err
		}
	}

	var res []containers.Container
	for _, containerGroup := range containerGroups {
		group, err := groupsClient.Get(ctx, cs.ctx.ResourceGroup, *containerGroup.Name)
		if err != nil {
			return []containers.Container{}, err
		}

		if _, ok := group.Tags[singleContainerTag]; ok {
			if group.Containers == nil || len(*group.Containers) < 1 {
				return []containers.Container{}, fmt.Errorf("no containers found in ACI container group %s", *containerGroup.Name)
			}
			container := (*group.Containers)[0]
			c := getContainer(*containerGroup.Name, group.IPAddress, container)
			res = append(res, c)
			continue
		}

		for _, container := range *group.Containers {
			var containerID string
			// don't list sidecar container
			if *container.Name == convert.ComposeDNSSidecarName {
				continue
			}
			containerID = *containerGroup.Name + composeContainerSeparator + *container.Name
			c := getContainer(containerID, group.IPAddress, container)
			res = append(res, c)
		}
	}

	return res, nil
}

func getContainer(containerID string, ipAddress *containerinstance.IPAddress, container containerinstance.Container) containers.Container {
	status := statusUnknown
	if container.InstanceView != nil && container.InstanceView.CurrentState != nil {
		status = *container.InstanceView.CurrentState.State
	}
	return containers.Container{
		ID:     containerID,
		Image:  *container.Image,
		Status: status,
		Ports:  convert.ToPorts(ipAddress, *container.Ports),
	}
}

func (cs *aciContainerService) Run(ctx context.Context, r containers.ContainerConfig) error {
	if strings.Contains(r.ID, composeContainerSeparator) {
		return errors.New(fmt.Sprintf("invalid container name. ACI container name cannot include %q", composeContainerSeparator))
	}

	project, err := convert.ContainerToComposeProject(r)
	if err != nil {
		return err
	}

	logrus.Debugf("Running container %q with name %q\n", r.Image, r.ID)
	groupDefinition, err := convert.ToContainerGroup(cs.ctx, project)
	if err != nil {
		return err
	}
	addTag(&groupDefinition, singleContainerTag)

	return createACIContainers(ctx, cs.ctx, groupDefinition)
}

func addTag(groupDefinition *containerinstance.ContainerGroup, tagName string) {
	if groupDefinition.Tags == nil {
		groupDefinition.Tags = make(map[string]*string, 1)
	}
	groupDefinition.Tags[tagName] = to.StringPtr(tagName)
}

func (cs *aciContainerService) Stop(ctx context.Context, containerName string, timeout *uint32) error {
	return errdefs.ErrNotImplemented
}

func getGroupAndContainerName(containerID string) (string, string) {
	tokens := strings.Split(containerID, composeContainerSeparator)
	groupName := tokens[0]
	containerName := groupName
	if len(tokens) > 1 {
		containerName = tokens[len(tokens)-1]
		groupName = containerID[:len(containerID)-(len(containerName)+1)]
	}
	return groupName, containerName
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

func (cs *aciContainerService) Delete(ctx context.Context, containerID string, _ bool) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot delete service %q from compose application %q, you can delete the entire compose app with docker compose down --project-name %s"
		return errors.New(fmt.Sprintf(msg, containerName, groupName, groupName))
	}
	cg, err := deleteACIContainerGroup(ctx, cs.ctx, groupName)
	if err != nil {
		return err
	}
	if cg.StatusCode == http.StatusNoContent {
		return ErrNoSuchContainer
	}

	return err
}

func (cs *aciContainerService) Inspect(ctx context.Context, containerID string) (containers.Container, error) {
	groupName, containerName := getGroupAndContainerName(containerID)

	cg, err := getACIContainerGroup(ctx, cs.ctx, groupName)
	if err != nil {
		return containers.Container{}, err
	}
	if cg.StatusCode == http.StatusNoContent {
		return containers.Container{}, ErrNoSuchContainer
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
		return containers.Container{}, ErrNoSuchContainer
	}

	return convert.ContainerGroupToContainer(containerID, cg, cc)
}

type aciComposeService struct {
	ctx store.AciContext
}

func (cs *aciComposeService) Up(ctx context.Context, opts cli.ProjectOptions) error {
	project, err := cli.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	logrus.Debugf("Up on project with name %q\n", project.Name)
	groupDefinition, err := convert.ToContainerGroup(cs.ctx, *project)
	addTag(&groupDefinition, composeContainerTag)

	if err != nil {
		return err
	}
	return createOrUpdateACIContainers(ctx, cs.ctx, groupDefinition)
}

func (cs *aciComposeService) Down(ctx context.Context, opts cli.ProjectOptions) error {
	var project types.Project

	if opts.Name != "" {
		project = types.Project{Name: opts.Name}
	} else {
		fullProject, err := cli.ProjectFromOptions(&opts)
		if err != nil {
			return err
		}
		project = *fullProject
	}
	logrus.Debugf("Down on project with name %q\n", project.Name)

	cg, err := deleteACIContainerGroup(ctx, cs.ctx, project.Name)
	if err != nil {
		return err
	}
	if cg.StatusCode == http.StatusNoContent {
		return ErrNoSuchContainer
	}

	return err
}

type aciCloudService struct {
	loginService login.AzureLoginService
}

func (cs *aciCloudService) Login(ctx context.Context, params interface{}) error {
	createOpts := params.(LoginParams)
	return cs.loginService.Login(ctx, createOpts.TenantID)
}

func (cs *aciCloudService) Logout(ctx context.Context) error {
	return cs.loginService.Logout(ctx)
}

func (cs *aciCloudService) CreateContextData(ctx context.Context, params interface{}) (interface{}, string, error) {
	contextHelper := newContextCreateHelper()
	createOpts := params.(ContextParams)
	return contextHelper.createContextData(ctx, createOpts)
}
