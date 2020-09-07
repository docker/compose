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

	"github.com/pkg/errors"

	"github.com/docker/compose-cli/context/store"
)

// StorageAccountHelper helper for Azure Storage Account
type StorageAccountHelper struct {
	LoginService AzureLoginService
	AciContext   store.AciContext
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

func (helper StorageAccountHelper) ListFileShare(ctx context.Context) ([]string, error) {
	aciContext := helper.AciContext
	accountClient, err := NewStorageAccountsClient(aciContext.SubscriptionID)
	if err != nil {
		return nil, err
	}
	result, err := accountClient.ListByResourceGroup(ctx, aciContext.ResourceGroup)
	accounts := result.Value
	fileShareClient, err := NewFileShareClient(aciContext.SubscriptionID)
	fileShares := []string{}
	for _, account := range *accounts {
		fileSharePage, err := fileShareClient.List(ctx, aciContext.ResourceGroup, *account.Name, "", "", "")
		if err != nil {
			return nil, err
		}
		for ; fileSharePage.NotDone() ; fileSharePage.NextWithContext(ctx) {
			values := fileSharePage.Values()
			for _, fileShare := range values {
				fileShares = append(fileShares, *fileShare.Name)
			}
		}
	}
	return fileShares, nil
}

func (helper StorageAccountHelper) CreateFileShare(ctx context.Context, accountName string, fileShareName string) (storage.FileShare, error) {
	aciContext := helper.AciContext
	accountClient, err := NewStorageAccountsClient(aciContext.SubscriptionID)
	if err != nil {
		return storage.FileShare{}, err
	}
	account, err := accountClient.GetProperties(ctx, aciContext.ResourceGroup, accountName, "")
	if err != nil {
		//TODO check err not found
		parameters := storage.AccountCreateParameters{
			Location: &aciContext.Location,
			Sku:&storage.Sku{
				Name: storage.StandardLRS,
				Tier: storage.Standard,
			},
		}
		// TODO progress account creation
		future, err := accountClient.Create(ctx, aciContext.ResourceGroup, accountName, parameters)
		if err != nil {
			return storage.FileShare{}, err
		}
		account, err = future.Result(accountClient)
	}
	fileShareClient, err := NewFileShareClient(aciContext.SubscriptionID)
	fileShare, err := fileShareClient.Get(ctx, aciContext.ResourceGroup, *account.Name, fileShareName, "")
	if err != nil {
		// TODO check err not found
		fileShare, err = fileShareClient.Create(ctx, aciContext.ResourceGroup, *account.Name, fileShareName, storage.FileShare{})
	}

	return fileShare, nil
}

