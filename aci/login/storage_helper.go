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

package login

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"

	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/errdefs"

	"github.com/pkg/errors"

	"github.com/docker/compose-cli/context/store"
)

// StorageAccountHelper helper for Azure Storage Account
type StorageAccountHelper struct {
	AciContext store.AciContext
}

// GetAzureStorageAccountKey retrieves the storage account ket from the current azure login
func (helper StorageAccountHelper) GetAzureStorageAccountKey(ctx context.Context, accountName string) (string, error) {
	client, err := NewStorageAccountsClient(helper.AciContext.SubscriptionID)
	if err != nil {
		return "", err
	}
	result, err := client.ListKeys(ctx, helper.AciContext.ResourceGroup, accountName, "")
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("could not access storage account acountKeys for %s, using the azure login", accountName))
	}
	if result.Keys != nil && len((*result.Keys)) < 1 {
		return "", fmt.Errorf("no key could be obtained for storage account %s from your azure login", accountName)
	}

	key := (*result.Keys)[0]
	return *key.Value, nil
}

// ListFileShare list file shares in all visible storage accounts
func (helper StorageAccountHelper) ListFileShare(ctx context.Context) ([]volumes.Volume, error) {
	aciContext := helper.AciContext
	accountClient, err := NewStorageAccountsClient(aciContext.SubscriptionID)
	if err != nil {
		return nil, err
	}
	result, err := accountClient.ListByResourceGroup(ctx, aciContext.ResourceGroup)
	if err != nil {
		return nil, err
	}
	accounts := result.Value
	fileShareClient, err := NewFileShareClient(aciContext.SubscriptionID)
	if err != nil {
		return nil, err
	}
	fileShares := []volumes.Volume{}
	for _, account := range *accounts {
		fileSharePage, err := fileShareClient.List(ctx, aciContext.ResourceGroup, *account.Name, "", "", "")
		if err != nil {
			return nil, err
		}

		for fileSharePage.NotDone() {
			values := fileSharePage.Values()
			for _, fileShare := range values {
				fileShares = append(fileShares, toVolume(account, *fileShare.Name))
			}
			if err := fileSharePage.NextWithContext(ctx); err != nil {
				return nil, err
			}
		}
	}
	return fileShares, nil
}

func toVolume(account storage.Account, fileShareName string) volumes.Volume {
	return volumes.Volume{
		ID:          fmt.Sprintf("%s@%s", *account.Name, fileShareName),
		Name:        fileShareName,
		Description: fmt.Sprintf("Fileshare %s in %s storage account", fileShareName, *account.Name),
	}
}

// CreateFileShare create a new fileshare
func (helper StorageAccountHelper) CreateFileShare(ctx context.Context, accountName string, fileShareName string) (volumes.Volume, error) {
	aciContext := helper.AciContext
	accountClient, err := NewStorageAccountsClient(aciContext.SubscriptionID)
	if err != nil {
		return volumes.Volume{}, err
	}
	account, err := accountClient.GetProperties(ctx, aciContext.ResourceGroup, accountName, "")
	if err != nil {
		if account.StatusCode != 404 {
			return volumes.Volume{}, err
		}
		//TODO confirm storage account creation
		parameters := defaultStorageAccountParams(aciContext)
		// TODO progress account creation
		future, err := accountClient.Create(ctx, aciContext.ResourceGroup, accountName, parameters)
		if err != nil {
			return volumes.Volume{}, err
		}
		account, err = future.Result(accountClient)
		if err != nil {
			return volumes.Volume{}, err
		}
	}
	fileShareClient, err := NewFileShareClient(aciContext.SubscriptionID)
	if err != nil {
		return volumes.Volume{}, err
	}

	fileShare, err := fileShareClient.Get(ctx, aciContext.ResourceGroup, *account.Name, fileShareName, "")
	if err == nil {
		return volumes.Volume{}, errors.Wrapf(errdefs.ErrAlreadyExists, "Azure fileshare %q already exists", fileShareName)
	}
	if fileShare.StatusCode != 404 {
		return volumes.Volume{}, err
	}
	fileShare, err = fileShareClient.Create(ctx, aciContext.ResourceGroup, *account.Name, fileShareName, storage.FileShare{})
	if err != nil {
		return volumes.Volume{}, err
	}
	return toVolume(account, *fileShare.Name), nil
}

func defaultStorageAccountParams(aciContext store.AciContext) storage.AccountCreateParameters {
	return storage.AccountCreateParameters{
		Location: &aciContext.Location,
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
			Tier: storage.Standard,
		},
	}
}
