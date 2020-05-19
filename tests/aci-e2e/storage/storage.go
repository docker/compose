package storage

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/storage/mgmt/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"

	"github.com/docker/api/azure/login"
	"github.com/docker/api/context/store"
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
		return storage.Account{}, err
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
