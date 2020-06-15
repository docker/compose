package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/pkg/errors"

	"github.com/docker/api/azure/login"
	"github.com/docker/api/errdefs"
)

// ACIResourceGroupHelper interface to manage resource groups and subscription IDs
type ACIResourceGroupHelper interface {
	GetSubscriptionIDs(ctx context.Context) ([]subscription.Model, error)
	ListGroups(ctx context.Context, subscriptionID string) ([]resources.Group, error)
	GetGroup(ctx context.Context, subscriptionID string, groupName string) (resources.Group, error)
	CreateOrUpdate(ctx context.Context, subscriptionID string, resourceGroupName string, parameters resources.Group) (result resources.Group, err error)
	Delete(ctx context.Context, subscriptionID string, resourceGroupName string) error
}

type aciResourceGroupHelperImpl struct {
}

// NewACIResourceGroupHelper create a new ACIResourceGroupHelper
func NewACIResourceGroupHelper() ACIResourceGroupHelper {
	return aciResourceGroupHelperImpl{}
}

// GetGroup get a resource group from its name
func (mgt aciResourceGroupHelperImpl) GetGroup(ctx context.Context, subscriptionID string, groupName string) (resources.Group, error) {
	gc := getGroupsClient(subscriptionID)
	return gc.Get(ctx, groupName)
}

// ListGroups list resource groups
func (mgt aciResourceGroupHelperImpl) ListGroups(ctx context.Context, subscriptionID string) ([]resources.Group, error) {
	gc := getGroupsClient(subscriptionID)
	groupResponse, err := gc.List(ctx, "", nil)
	if err != nil {
		return nil, err
	}

	groups := groupResponse.Values()
	return groups, nil
}

// CreateOrUpdate create or update a resource group
func (mgt aciResourceGroupHelperImpl) CreateOrUpdate(ctx context.Context, subscriptionID string, resourceGroupName string, parameters resources.Group) (result resources.Group, err error) {
	gc := getGroupsClient(subscriptionID)
	return gc.CreateOrUpdate(ctx, resourceGroupName, parameters)
}

// Delete deletes a resource group
func (mgt aciResourceGroupHelperImpl) Delete(ctx context.Context, subscriptionID string, resourceGroupName string) (err error) {
	gc := getGroupsClient(subscriptionID)
	future, err := gc.Delete(ctx, resourceGroupName)
	if err != nil {
		return err
	}
	return future.WaitForCompletionRef(ctx, gc.Client)
}

// GetSubscriptionIDs Return available subscription IDs based on azure login
func (mgt aciResourceGroupHelperImpl) GetSubscriptionIDs(ctx context.Context) ([]subscription.Model, error) {
	c, err := getSubscriptionsClient()
	if err != nil {
		return nil, err
	}
	res, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	subs := res.Values()

	if len(subs) == 0 {
		return nil, errors.New("no subscriptions found")
	}
	for res.NotDone() {
		err = res.NextWithContext(ctx)
		if err != nil {
			return nil, err
		}
		subs = append(subs, res.Values()...)
	}
	return subs, nil
}

func getSubscriptionsClient() (subscription.SubscriptionsClient, error) {
	subc := subscription.NewSubscriptionsClient()
	authorizer, err := login.NewAuthorizerFromLogin()
	if err != nil {
		return subscription.SubscriptionsClient{}, errors.Wrap(errdefs.ErrLoginFailed, err.Error())
	}
	subc.Authorizer = authorizer
	subc.UserAgent=aciDockerUserAgent
	return subc, nil
}

func getGroupsClient(subscriptionID string) resources.GroupsClient {
	groupsClient := resources.NewGroupsClient(subscriptionID)
	authorizer, _ := login.NewAuthorizerFromLogin()
	groupsClient.Authorizer = authorizer
	groupsClient.UserAgent=aciDockerUserAgent
	return groupsClient
}
