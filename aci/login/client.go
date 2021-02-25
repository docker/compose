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

package login

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/internal"
)

// NewContainerGroupsClient get client toi manipulate containerGrouos
func NewContainerGroupsClient(subscriptionID string) (containerinstance.ContainerGroupsClient, error) {
	authorizer, mgmtURL, err := getClientSetupData()
	if err != nil {
		return containerinstance.ContainerGroupsClient{}, err
	}
	containerGroupsClient := containerinstance.NewContainerGroupsClientWithBaseURI(mgmtURL, subscriptionID)
	setupClient(&containerGroupsClient.Client, authorizer)
	if err != nil {
		return containerinstance.ContainerGroupsClient{}, err
	}
	containerGroupsClient.PollingDelay = 5 * time.Second
	containerGroupsClient.RetryAttempts = 30
	containerGroupsClient.RetryDuration = 1 * time.Second
	return containerGroupsClient, nil
}

func setupClient(aciClient *autorest.Client, auth autorest.Authorizer) {
	aciClient.UserAgent = internal.UserAgentName + "/" + internal.Version
	aciClient.Authorizer = auth
}

// NewStorageAccountsClient get client to manipulate storage accounts
func NewStorageAccountsClient(subscriptionID string) (storage.AccountsClient, error) {
	authorizer, mgmtURL, err := getClientSetupData()
	if err != nil {
		return storage.AccountsClient{}, err
	}
	storageAccuntsClient := storage.NewAccountsClientWithBaseURI(mgmtURL, subscriptionID)
	setupClient(&storageAccuntsClient.Client, authorizer)
	storageAccuntsClient.PollingDelay = 5 * time.Second
	storageAccuntsClient.RetryAttempts = 30
	storageAccuntsClient.RetryDuration = 1 * time.Second
	return storageAccuntsClient, nil
}

// NewFileShareClient get client to manipulate file shares
func NewFileShareClient(subscriptionID string) (storage.FileSharesClient, error) {
	authorizer, mgmtURL, err := getClientSetupData()
	if err != nil {
		return storage.FileSharesClient{}, err
	}
	fileSharesClient := storage.NewFileSharesClientWithBaseURI(mgmtURL, subscriptionID)
	setupClient(&fileSharesClient.Client, authorizer)
	fileSharesClient.PollingDelay = 5 * time.Second
	fileSharesClient.RetryAttempts = 30
	fileSharesClient.RetryDuration = 1 * time.Second
	return fileSharesClient, nil
}

// NewSubscriptionsClient get subscription client
func NewSubscriptionsClient() (subscription.SubscriptionsClient, error) {
	authorizer, mgmtURL, err := getClientSetupData()
	if err != nil {
		return subscription.SubscriptionsClient{}, errors.Wrap(errdefs.ErrLoginRequired, err.Error())
	}
	subc := subscription.NewSubscriptionsClientWithBaseURI(mgmtURL)
	setupClient(&subc.Client, authorizer)
	return subc, nil
}

// NewGroupsClient get client to manipulate groups
func NewGroupsClient(subscriptionID string) (resources.GroupsClient, error) {
	authorizer, mgmtURL, err := getClientSetupData()
	if err != nil {
		return resources.GroupsClient{}, err
	}
	groupsClient := resources.NewGroupsClientWithBaseURI(mgmtURL, subscriptionID)
	setupClient(&groupsClient.Client, authorizer)
	return groupsClient, nil
}

// NewContainerClient get client to manipulate containers
func NewContainerClient(subscriptionID string) (containerinstance.ContainersClient, error) {
	authorizer, mgmtURL, err := getClientSetupData()
	if err != nil {
		return containerinstance.ContainersClient{}, err
	}
	containerClient := containerinstance.NewContainersClientWithBaseURI(mgmtURL, subscriptionID)
	setupClient(&containerClient.Client, authorizer)
	return containerClient, nil
}

func getClientSetupData() (autorest.Authorizer, string, error) {
	return getClientSetupDataImpl(GetTokenStorePath())
}

func getClientSetupDataImpl(tokenStorePath string) (autorest.Authorizer, string, error) {
	als, err := newAzureLoginServiceFromPath(tokenStorePath, azureAPIHelper{}, CloudEnvironments)
	if err != nil {
		return nil, "", err
	}

	oauthToken, _, err := als.GetValidToken()
	if err != nil {
		return nil, "", errors.Wrap(err, "not logged in to azure, you need to run \"docker login azure\" first")
	}

	ce, err := als.GetCloudEnvironment()
	if err != nil {
		return nil, "", err
	}

	token := adal.Token{
		AccessToken:  oauthToken.AccessToken,
		Type:         oauthToken.TokenType,
		ExpiresIn:    json.Number(strconv.Itoa(int(time.Until(oauthToken.Expiry).Seconds()))),
		ExpiresOn:    json.Number(strconv.Itoa(int(oauthToken.Expiry.Sub(date.UnixEpoch()).Seconds()))),
		RefreshToken: "",
		Resource:     "",
	}

	return autorest.NewBearerAuthorizer(&token), ce.ResourceManagerURL, nil
}
