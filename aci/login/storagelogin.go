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
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/context/store"
)

// StorageLogin helper for Azure Storage Login
type StorageLogin interface {
	// GetAzureStorageAccountKey retrieves the storage account ket from the current azure login
	GetAzureStorageAccountKey(ctx context.Context, accountName string) (string, error)
}

// StorageLoginImpl implementation of StorageLogin
type StorageLoginImpl struct {
	AciContext store.AciContext
}

// GetAzureStorageAccountKey retrieves the storage account ket from the current azure login
func (helper StorageLoginImpl) GetAzureStorageAccountKey(ctx context.Context, accountName string) (string, error) {
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
