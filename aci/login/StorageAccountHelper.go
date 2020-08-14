package login

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/docker/api/context/store"
)

// StorageAccountHelper helper for Azure Storage Account
type StorageAccountHelper struct {
	LoginService AzureLoginService
	AciContext   store.AciContext
}

// GetAzureStorageAccountKey retrieves the storage account ket from the current azure login
func (helper StorageAccountHelper) GetAzureStorageAccountKey(ctx context.Context, accountName string) (string, error) {
	client, err := GetStorageAccountsClient(helper.AciContext.SubscriptionID)
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
