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

package storage

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/storage/mgmt/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"

	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/context/store"
)

// CreateStorageAccount creates a new storage account.
func CreateStorageAccount(ctx context.Context, aciContext store.AciContext, accountName string) (storage.Account, error) {
	storageAccountsClient := getStorageAccountsClient(aciContext)
	result, err := storageAccountsClient.CheckNameAvailability(
		ctx,
		storage.AccountCheckNameAvailabilityParameters{
			Name: to.StringPtr(accountName),
			Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
		})

	if err != nil {
		return storage.Account{}, err
	}
	if !*result.NameAvailable {
		return storage.Account{}, errors.New("storage account name already exists" + accountName)
	}

	future, err := storageAccountsClient.Create(
		ctx,
		aciContext.ResourceGroup,
		accountName,
		storage.AccountCreateParameters{
			Sku: &storage.Sku{
				Name: storage.StandardLRS,
			},
			Location:                          to.StringPtr(aciContext.Location),
			AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{}})
	if err != nil {
		return storage.Account{}, err
	}
	err = future.WaitForCompletionRef(ctx, storageAccountsClient.Client)
	if err != nil {
		return storage.Account{}, err
	}
	return future.Result(storageAccountsClient)
}

// DeleteStorageAccount deletes a given storage account
func DeleteStorageAccount(ctx context.Context, aciContext store.AciContext, accountName string) (autorest.Response, error) {
	storageAccountsClient := getStorageAccountsClient(aciContext)
	response, err := storageAccountsClient.Delete(ctx, aciContext.ResourceGroup, accountName)
	if err != nil {
		return autorest.Response{}, err
	}
	return response, err
}

// ListKeys lists the storage account keys
func ListKeys(ctx context.Context, aciContext store.AciContext, accountName string) (storage.AccountListKeysResult, error) {
	storageAccountsClient := getStorageAccountsClient(aciContext)
	keys, err := storageAccountsClient.ListKeys(ctx, aciContext.ResourceGroup, accountName)
	if err != nil {
		return storage.AccountListKeysResult{}, err
	}
	return keys, nil
}

func getStorageAccountsClient(aciContext store.AciContext) storage.AccountsClient {
	storageAccountsClient := storage.NewAccountsClient(aciContext.SubscriptionID)
	autho, _ := login.NewAuthorizerFromLogin()
	storageAccountsClient.Authorizer = autho
	return storageAccountsClient
}
