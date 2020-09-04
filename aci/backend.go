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
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/backend"
	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
)

const (
	backendType               = store.AciContextType
	singleContainerTag        = "docker-single-container"
	composeContainerTag       = "docker-compose-application"
	composeContainerSeparator = "_"
)

// ContextParams options for creating ACI context
type ContextParams struct {
	Description    string
	Location       string
	SubscriptionID string
	ResourceGroup  string
}

// LoginParams azure login options
type LoginParams struct {
	TenantID     string
	ClientID     string
	ClientSecret string
}

// Validate returns an error if options are not used properly
func (opts LoginParams) Validate() error {
	if opts.ClientID != "" || opts.ClientSecret != "" {
		if opts.ClientID == "" || opts.ClientSecret == "" || opts.TenantID == "" {
			return errors.New("for Service Principal login, 3 options must be specified: --client-id, --client-secret and --tenant-id")
		}
	}
	return nil
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

func (a *aciAPIService) SecretsService() secrets.Service {
	return nil
}

type aciContainerService struct {
	ctx store.AciContext
}

func (cs *aciContainerService) List(ctx context.Context, all bool) ([]containers.Container, error) {
	containerGroups, err := getContainerGroups(ctx, cs.ctx.SubscriptionID, cs.ctx.ResourceGroup)
	if err != nil {
		return []containers.Container{}, err
	}
	var res []containers.Container
	for _, group := range containerGroups {
		if group.Containers == nil || len(*group.Containers) < 1 {
			return []containers.Container{}, fmt.Errorf("no containers found in ACI container group %s", *group.Name)
		}

		for _, container := range *group.Containers {
			if isContainerVisible(container, group, all) {
				continue
			}
			c := convert.ContainerGroupToContainer(getContainerID(group, container), group, container)
			res = append(res, c)
		}
	}
	return res, nil
}

func getContainerGroups(ctx context.Context, subscriptionID string, resourceGroup string) ([]containerinstance.ContainerGroup, error) {
	groupsClient, err := login.NewContainerGroupsClient(subscriptionID)
	if err != nil {
		return nil, err
	}
	var containerGroups []containerinstance.ContainerGroup
	result, err := groupsClient.ListByResourceGroup(ctx, resourceGroup)
	if err != nil {
		return []containerinstance.ContainerGroup{}, err
	}

	for result.NotDone() {
		containerGroups = append(containerGroups, result.Values()...)
		if err := result.NextWithContext(ctx); err != nil {
			return []containerinstance.ContainerGroup{}, err
		}
	}
	return containerGroups, nil
}

func getContainerID(group containerinstance.ContainerGroup, container containerinstance.Container) string {
	containerID := *group.Name + composeContainerSeparator + *container.Name
	if _, ok := group.Tags[singleContainerTag]; ok {
		containerID = *group.Name
	}
	return containerID
}

func isContainerVisible(container containerinstance.Container, group containerinstance.ContainerGroup, showAll bool) bool {
	return *container.Name == convert.ComposeDNSSidecarName || (!showAll && convert.GetStatus(container, group) != convert.StatusRunning)
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
	groupDefinition, err := convert.ToContainerGroup(ctx, cs.ctx, project)
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

func (cs *aciContainerService) Start(ctx context.Context, containerID string) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot start specified service %q from compose application %q, you can update and restart the entire compose app with docker compose up --project-name %s"
		return errors.New(fmt.Sprintf(msg, containerName, groupName, groupName))
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
		return errors.Errorf("ACI integration does not support setting a timeout to stop a container before killing it.")
	}
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot stop service %q from compose application %q, you can stop the entire compose app with docker stop %s"
		return errors.New(fmt.Sprintf(msg, containerName, groupName, groupName))
	}
	return stopACIContainerGroup(ctx, cs.ctx, groupName)
}

func (cs *aciContainerService) Kill(ctx context.Context, containerID string, _ string) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot kill service %q from compose application %q, you can kill the entire compose app with docker kill %s"
		return errors.New(fmt.Sprintf(msg, containerName, groupName, groupName))
	}
	return stopACIContainerGroup(ctx, cs.ctx, groupName) // As ACI doesn't have a kill command, we are using the stop implementation instead
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

func (cs *aciContainerService) Delete(ctx context.Context, containerID string, request containers.DeleteRequest) error {
	groupName, containerName := getGroupAndContainerName(containerID)
	if groupName != containerID {
		msg := "cannot delete service %q from compose application %q, you can delete the entire compose app with docker compose down --project-name %s"
		return errors.New(fmt.Sprintf(msg, containerName, groupName, groupName))
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
	if cg.StatusCode == http.StatusNoContent {
		return errdefs.ErrNotFound
	}
	if err != nil {
		return err
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

	return convert.ContainerGroupToContainer(containerID, cg, cc), nil
}

type aciComposeService struct {
	ctx store.AciContext
}

func (cs *aciComposeService) Up(ctx context.Context, project *types.Project) error {
	logrus.Debugf("Up on project with name %q\n", project.Name)
	groupDefinition, err := convert.ToContainerGroup(ctx, cs.ctx, *project)
	addTag(&groupDefinition, composeContainerTag)

	if err != nil {
		return err
	}
	return createOrUpdateACIContainers(ctx, cs.ctx, groupDefinition)
}

func (cs *aciComposeService) Down(ctx context.Context, project string) error {
	logrus.Debugf("Down on project with name %q\n", project)

	cg, err := deleteACIContainerGroup(ctx, cs.ctx, project)
	if err != nil {
		return err
	}
	if cg.StatusCode == http.StatusNoContent {
		return errdefs.ErrNotFound
	}

	return err
}

func (cs *aciComposeService) Ps(ctx context.Context, project string) ([]compose.ServiceStatus, error) {
	groupsClient, err := login.NewContainerGroupsClient(cs.ctx.SubscriptionID)
	if err != nil {
		return nil, err
	}

	group, err := groupsClient.Get(ctx, cs.ctx.ResourceGroup, project)
	if err != nil {
		return []compose.ServiceStatus{}, err
	}

	if group.Containers == nil || len(*group.Containers) < 1 {
		return []compose.ServiceStatus{}, fmt.Errorf("no containers found in ACI container group %s", project)
	}

	res := []compose.ServiceStatus{}
	for _, container := range *group.Containers {
		if isContainerVisible(container, group, false) {
			continue
		}
		res = append(res, convert.ContainerGroupToServiceStatus(getContainerID(group, container), group, container))
	}
	return res, nil
}

func (cs *aciComposeService) List(ctx context.Context, project string) ([]compose.Stack, error) {
	containerGroups, err := getContainerGroups(ctx, cs.ctx.SubscriptionID, cs.ctx.ResourceGroup)
	if err != nil {
		return []compose.Stack{}, err
	}

	stacks := []compose.Stack{}
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

func (cs *aciComposeService) Logs(ctx context.Context, project string, w io.Writer) error {
	return errdefs.ErrNotImplemented
}

func (cs *aciComposeService) Convert(ctx context.Context, project *types.Project) ([]byte, error) {
	return nil, errdefs.ErrNotImplemented
}

type aciCloudService struct {
	loginService login.AzureLoginServiceAPI
}

func (cs *aciCloudService) Login(ctx context.Context, params interface{}) error {
	opts, ok := params.(LoginParams)
	if !ok {
		return errors.New("Could not read azure LoginParams struct from generic parameter")
	}
	if opts.ClientID != "" {
		return cs.loginService.LoginServicePrincipal(opts.ClientID, opts.ClientSecret, opts.TenantID)
	}
	return cs.loginService.Login(ctx, opts.TenantID)
}

func (cs *aciCloudService) Logout(ctx context.Context) error {
	return cs.loginService.Logout(ctx)
}

func (cs *aciCloudService) CreateContextData(ctx context.Context, params interface{}) (interface{}, string, error) {
	contextHelper := newContextCreateHelper()
	createOpts := params.(ContextParams)
	return contextHelper.createContextData(ctx, createOpts)
}
