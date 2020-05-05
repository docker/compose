package azure

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/docker/api/azure/convert"
	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

func init() {
	backend.Register("aci", "aci", func(ctx context.Context) (interface{}, error) {
		return New(ctx)
	})
}

func getter() interface{} {
	return &store.AciContext{}
}

// New creates a backend that can manage containers
func New(ctx context.Context) (backend.Service, error) {
	currentContext := apicontext.CurrentContext(ctx)
	contextStore, err := store.New()
	if err != nil {
		return nil, err
	}
	metadata, err := contextStore.Get(currentContext, getter)
	if err != nil {
		return nil, errors.Wrap(err, "wrong context type")
	}
	aciContext, _ := metadata.Metadata.Data.(store.AciContext)

	auth, _ := auth.NewAuthorizerFromCLI()
	containerGroupsClient := containerinstance.NewContainerGroupsClient(aciContext.SubscriptionID)
	containerGroupsClient.Authorizer = auth

	return getAciAPIService(containerGroupsClient, aciContext), nil
}

func getAciAPIService(cgc containerinstance.ContainerGroupsClient, aciCtx store.AciContext) *aciAPIService {
	return &aciAPIService{
		container: aciContainerService{
			containerGroupsClient: cgc,
			ctx:                   aciCtx,
		},
		compose: aciComposeService{
			containerGroupsClient: cgc,
			ctx:                   aciCtx,
		},
	}
}

type aciAPIService struct {
	container aciContainerService
	compose   aciComposeService
}

func (a *aciAPIService) ContainerService() containers.Service {
	return &aciContainerService{
		containerGroupsClient: a.container.containerGroupsClient,
		ctx:                   a.container.ctx,
	}
}

func (a *aciAPIService) ComposeService() compose.Service {
	return &aciComposeService{
		containerGroupsClient: a.compose.containerGroupsClient,
		ctx:                   a.compose.ctx,
	}
}

type aciContainerService struct {
	containerGroupsClient containerinstance.ContainerGroupsClient
	ctx                   store.AciContext
}

func (cs *aciContainerService) List(ctx context.Context) ([]containers.Container, error) {
	var containerGroups []containerinstance.ContainerGroup
	result, err := cs.containerGroupsClient.ListByResourceGroup(ctx, cs.ctx.ResourceGroup)
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
		group, err := cs.containerGroupsClient.Get(ctx, cs.ctx.ResourceGroup, *containerGroup.Name)
		if err != nil {
			return []containers.Container{}, err
		}

		for _, container := range *group.Containers {
			status := "Unknown"
			if container.InstanceView != nil && container.InstanceView.CurrentState != nil {
				status = *container.InstanceView.CurrentState.State
			}
			res = append(res, containers.Container{
				ID:     *container.Name,
				Image:  *container.Image,
				Status: status,
			})
		}
	}

	return res, nil
}

func (cs *aciContainerService) Run(ctx context.Context, r containers.ContainerConfig) error {
	var ports []types.ServicePortConfig
	for _, p := range r.Ports {
		ports = append(ports, types.ServicePortConfig{
			Target:    p.Destination,
			Published: p.Source,
		})
	}
	project := compose.Project{
		Name: r.ID,
		Config: types.Config{
			Services: []types.ServiceConfig{
				{
					Name:  r.ID,
					Image: r.Image,
					Ports: ports,
				},
			},
		},
	}

	logrus.Debugf("Running container %q with name %q\n", r.Image, r.ID)
	groupDefinition, err := convert.ToContainerGroup(cs.ctx, project)
	if err != nil {
		return err
	}

	return createACIContainers(ctx, cs.ctx, groupDefinition)
}

func (cs *aciContainerService) Exec(ctx context.Context, name string, command string, reader io.Reader, writer io.Writer) error {
	containerExecResponse, err := execACIContainer(ctx, cs.ctx, command, name, name)
	if err != nil {
		return err
	}

	return exec(
		context.Background(),
		*containerExecResponse.WebSocketURI,
		*containerExecResponse.Password,
		reader,
		writer,
	)
}

func (cs *aciContainerService) Logs(ctx context.Context, containerName string, req containers.LogsRequest) error {
	logs, err := getACIContainerLogs(ctx, cs.ctx, containerName, containerName)
	if err != nil {
		return err
	}
	if req.Tail != "all" {
		tail, err := strconv.Atoi(req.Tail)
		if err != nil {
			return err
		}
		lines := strings.Split(logs, "\n")

		// If asked for less lines than exist, take only those lines
		if tail <= len(lines) {
			logs = strings.Join(lines[len(lines)-tail:], "\n")
		}
	}

	_, err = fmt.Fprint(req.Writer, logs)
	return err
}

type aciComposeService struct {
	containerGroupsClient containerinstance.ContainerGroupsClient
	ctx                   store.AciContext
}

func (cs *aciComposeService) Up(ctx context.Context, opts compose.ProjectOptions) error {
	project, err := compose.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	logrus.Debugf("Up on project with name %q\n", project.Name)
	groupDefinition, err := convert.ToContainerGroup(cs.ctx, *project)
	if err != nil {
		return err
	}
	return createACIContainers(ctx, cs.ctx, groupDefinition)
}

func (cs *aciComposeService) Down(ctx context.Context, opts compose.ProjectOptions) error {
	project, err := compose.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	logrus.Debugf("Down on project with name %q\n", project.Name)
	_, err = deleteACIContainerGroup(ctx, cs.ctx, project.Name)
	return err
}
