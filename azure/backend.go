package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/pkg/errors"

	"github.com/docker/api/backend"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

type containerService struct {
	cgc containerinstance.ContainerGroupsClient
	ctx store.AciContext
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
	cc := apicontext.CurrentContext(ctx)
	contextStore, err := store.New()
	if err != nil {
		return nil, err
	}
	metadata, err := contextStore.Get(cc, getter)
	if err != nil {
		return nil, errors.Wrap(err, "wrong context type")
	}
	tc, _ := metadata.Metadata.Data.(store.AciContext)

	auth, _ := auth.NewAuthorizerFromCLI()
	containerGroupsClient := containerinstance.NewContainerGroupsClient(tc.SubscriptionID)
	containerGroupsClient.Authorizer = auth

	return &containerService{
		cgc: containerGroupsClient,
		ctx: tc,
	}, nil
}

func (cs *containerService) List(ctx context.Context) ([]containers.Container, error) {
	var cg []containerinstance.ContainerGroup
	result, err := cs.cgc.ListByResourceGroup(ctx, cs.ctx.ResourceGroup)
	if err != nil {
		return []containers.Container{}, err
	}

	for result.NotDone() {
		cg = append(cg, result.Values()...)
		if err := result.NextWithContext(ctx); err != nil {
			return []containers.Container{}, err
		}
	}

	res := []containers.Container{}
	for _, c := range cg {
		group, err := cs.cgc.Get(ctx, cs.ctx.ResourceGroup, *c.Name)
		if err != nil {
			return []containers.Container{}, err
		}

		for _, d := range *group.Containers {
			status := "Unknown"
			if d.InstanceView != nil && d.InstanceView.CurrentState != nil {
				status = *d.InstanceView.CurrentState.State
			}
			res = append(res, containers.Container{
				ID:     *d.Name,
				Image:  *d.Image,
				Status: status,
			})
		}
	}

	return res, nil
}
