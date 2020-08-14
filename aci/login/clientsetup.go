package login

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/pkg/errors"

	"github.com/docker/api/errdefs"
)

const aciDockerUserAgent = "docker-cli"

// GetContainerGroupsClient get client toi manipulate containerGrouos
func GetContainerGroupsClient(subscriptionID string) (containerinstance.ContainerGroupsClient, error) {
	containerGroupsClient := containerinstance.NewContainerGroupsClient(subscriptionID)
	err := setupClient(&containerGroupsClient.Client)
	if err != nil {
		return containerinstance.ContainerGroupsClient{}, err
	}
	containerGroupsClient.PollingDelay = 5 * time.Second
	containerGroupsClient.RetryAttempts = 30
	containerGroupsClient.RetryDuration = 1 * time.Second
	return containerGroupsClient, nil
}

func setupClient(aciClient *autorest.Client) error {
	aciClient.UserAgent = aciDockerUserAgent
	auth, err := NewAuthorizerFromLogin()
	if err != nil {
		return err
	}
	aciClient.Authorizer = auth
	return nil
}

// GetStorageAccountsClient get client to manipulate storage accounts
func GetStorageAccountsClient(subscriptionID string) (storage.AccountsClient, error) {
	containerGroupsClient := storage.NewAccountsClient(subscriptionID)
	err := setupClient(&containerGroupsClient.Client)
	if err != nil {
		return storage.AccountsClient{}, err
	}
	containerGroupsClient.PollingDelay = 5 * time.Second
	containerGroupsClient.RetryAttempts = 30
	containerGroupsClient.RetryDuration = 1 * time.Second
	return containerGroupsClient, nil
}

// GetSubscriptionsClient get subscription client
func GetSubscriptionsClient() (subscription.SubscriptionsClient, error) {
	subc := subscription.NewSubscriptionsClient()
	err := setupClient(&subc.Client)
	if err != nil {
		return subscription.SubscriptionsClient{}, errors.Wrap(errdefs.ErrLoginRequired, err.Error())
	}
	return subc, nil
}

// GetGroupsClient get client to manipulate groups
func GetGroupsClient(subscriptionID string) (resources.GroupsClient, error) {
	groupsClient := resources.NewGroupsClient(subscriptionID)
	err := setupClient(&groupsClient.Client)
	if err != nil {
		return resources.GroupsClient{}, err
	}
	return groupsClient, nil
}

// GetContainerClient get client to manipulate containers
func GetContainerClient(subscriptionID string) (containerinstance.ContainerClient, error) {
	containerClient := containerinstance.NewContainerClient(subscriptionID)
	err := setupClient(&containerClient.Client)
	if err != nil {
		return containerinstance.ContainerClient{}, err
	}
	return containerClient, nil
}
