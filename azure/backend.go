package azure

import (
	"context"

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

type containerService struct {
	containerGroupsClient containerinstance.ContainerGroupsClient
	ctx                   store.AciContext
}

func init() {
	backend.Register("aci", "aci", func(ctx context.Context) (interface{}, error) {
		return New(ctx)
	})
}

func getter() interface{} {
	return &store.AciContext{}
}

func New(ctx context.Context) (containers.ContainerService, error) {
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

	return &containerService{
		containerGroupsClient: containerGroupsClient,
		ctx:                   aciContext,
	}, nil
}

func (cs *containerService) List(ctx context.Context) ([]containers.Container, error) {
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

	res := []containers.Container{}
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

func (cs *containerService) Run(ctx context.Context, r containers.ContainerConfig) error {
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

	_, err = createACIContainers(ctx, cs.ctx, groupDefinition)
	return err
}
