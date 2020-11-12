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

package aci

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	"github.com/Azure/go-autorest/autorest/to"

	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/progress"
)

type aciVolumeService struct {
	aciContext store.AciContext
}

func (cs *aciVolumeService) List(ctx context.Context) ([]volumes.Volume, error) {
	accountClient, err := login.NewStorageAccountsClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return nil, err
	}
	result, err := accountClient.ListByResourceGroup(ctx, cs.aciContext.ResourceGroup)
	if err != nil {
		return nil, err
	}
	accounts := result.Value
	fileShareClient, err := login.NewFileShareClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return nil, err
	}
	fileShares := []volumes.Volume{}
	for _, account := range *accounts {
		fileSharePage, err := fileShareClient.List(ctx, cs.aciContext.ResourceGroup, *account.Name, "", "", "")
		if err != nil {
			return nil, err
		}

		for fileSharePage.NotDone() {
			values := fileSharePage.Values()
			for _, fileShare := range values {
				fileShares = append(fileShares, toVolume(*account.Name, *fileShare.Name))
			}
			if err := fileSharePage.NextWithContext(ctx); err != nil {
				return nil, err
			}
		}
	}
	return fileShares, nil
}

// VolumeCreateOptions options to create a new ACI volume
type VolumeCreateOptions struct {
	Account string
}

func (cs *aciVolumeService) Create(ctx context.Context, name string, options interface{}) (volumes.Volume, error) {
	opts, ok := options.(*VolumeCreateOptions)
	if !ok || opts == nil {
		return volumes.Volume{}, errors.New("could not read Azure VolumeCreateOptions struct from generic parameter")
	}
	w := progress.ContextWriter(ctx)
	w.Event(event(opts.Account, progress.Working, "Validating"))
	accountClient, err := login.NewStorageAccountsClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return volumes.Volume{}, err
	}
	account, err := accountClient.GetProperties(ctx, cs.aciContext.ResourceGroup, opts.Account, "")
	if err == nil {
		w.Event(event(opts.Account, progress.Done, "Use existing"))
	} else if !account.HasHTTPStatus(http.StatusNotFound) {
		return volumes.Volume{}, err
	} else {
		result, err := accountClient.CheckNameAvailability(ctx, storage.AccountCheckNameAvailabilityParameters{
			Name: to.StringPtr(opts.Account),
			Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
		})
		if err != nil {
			return volumes.Volume{}, err
		}
		if !*result.NameAvailable {
			return volumes.Volume{}, errors.New("error: " + *result.Message)
		}
		parameters := defaultStorageAccountParams(cs.aciContext)

		w.Event(event(opts.Account, progress.Working, "Creating"))

		future, err := accountClient.Create(ctx, cs.aciContext.ResourceGroup, opts.Account, parameters)
		if err != nil {
			w.Event(errorEvent(opts.Account))
			return volumes.Volume{}, err
		}
		if err := future.WaitForCompletionRef(ctx, accountClient.Client); err != nil {
			w.Event(errorEvent(opts.Account))
			return volumes.Volume{}, err
		}
		account, err = future.Result(accountClient)
		if err != nil {
			w.Event(errorEvent(opts.Account))
			return volumes.Volume{}, err
		}
		w.Event(event(opts.Account, progress.Done, "Created"))
	}
	w.Event(event(name, progress.Working, "Creating"))
	fileShareClient, err := login.NewFileShareClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return volumes.Volume{}, err
	}

	fileShare, err := fileShareClient.Get(ctx, cs.aciContext.ResourceGroup, *account.Name, name, "")
	if err == nil {
		w.Event(errorEvent(name))
		return volumes.Volume{}, errors.Wrapf(errdefs.ErrAlreadyExists, "Azure fileshare %q already exists", name)
	}
	if !fileShare.HasHTTPStatus(http.StatusNotFound) {
		w.Event(errorEvent(name))
		return volumes.Volume{}, err
	}
	fileShare, err = fileShareClient.Create(ctx, cs.aciContext.ResourceGroup, *account.Name, name, storage.FileShare{})
	if err != nil {
		w.Event(errorEvent(name))
		return volumes.Volume{}, err
	}
	w.Event(event(name, progress.Done, "Created"))
	return toVolume(*account.Name, *fileShare.Name), nil
}

func event(resource string, status progress.EventStatus, text string) progress.Event {
	return progress.Event{
		ID:         resource,
		Status:     status,
		StatusText: text,
	}
}

func errorEvent(resource string) progress.Event {
	return progress.Event{
		ID:         resource,
		Status:     progress.Error,
		StatusText: "Error",
	}
}

func checkVolumeUsage(ctx context.Context, aciContext store.AciContext, id string) error {
	containerGroups, err := getACIContainerGroups(ctx, aciContext.SubscriptionID, aciContext.ResourceGroup)
	if err != nil {
		return err
	}
	for _, cg := range containerGroups {
		if hasVolume(cg.Volumes, id) {
			return errors.Errorf("volume %q is used in container group %q",
				id, *cg.Name)
		}
	}
	return nil
}

func hasVolume(volumes *[]containerinstance.Volume, id string) bool {
	if volumes == nil {
		return false
	}
	for _, v := range *volumes {
		if v.AzureFile != nil && v.AzureFile.StorageAccountName != nil && v.AzureFile.ShareName != nil &&
			(*v.AzureFile.StorageAccountName+"/"+*v.AzureFile.ShareName) == id {
			return true
		}
	}
	return false
}

func (cs *aciVolumeService) Delete(ctx context.Context, id string, options interface{}) error {
	err := checkVolumeUsage(ctx, cs.aciContext, id)
	if err != nil {
		return err
	}
	storageAccount, fileshare, err := getStorageAccountAndFileshare(id)
	if err != nil {
		return err
	}

	fileShareClient, err := login.NewFileShareClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return err
	}
	fileShareItemsPage, err := fileShareClient.List(ctx, cs.aciContext.ResourceGroup, storageAccount, "", "", "")
	if err != nil {
		return err
	}
	fileshares := fileShareItemsPage.Values()
	if len(fileshares) == 1 && *fileshares[0].Name == fileshare {
		storageAccountsClient, err := login.NewStorageAccountsClient(cs.aciContext.SubscriptionID)
		if err != nil {
			return err
		}
		account, err := storageAccountsClient.GetProperties(ctx, cs.aciContext.ResourceGroup, storageAccount, "")
		if err != nil {
			return err
		}
		if err == nil {
			if _, ok := account.Tags[dockerVolumeTag]; ok {
				result, err := storageAccountsClient.Delete(ctx, cs.aciContext.ResourceGroup, storageAccount)
				if result.IsHTTPStatus(http.StatusNoContent) {
					return errors.Wrapf(errdefs.ErrNotFound, "storage account %s does not exist", storageAccount)
				}
				return err
			}
		}
	}

	result, err := fileShareClient.Delete(ctx, cs.aciContext.ResourceGroup, storageAccount, fileshare)
	if result.StatusCode == 204 {
		return errors.Wrapf(errdefs.ErrNotFound, "fileshare %q", fileshare)
	}
	return err
}

func (cs *aciVolumeService) Inspect(ctx context.Context, id string) (volumes.Volume, error) {
	storageAccount, fileshareName, err := getStorageAccountAndFileshare(id)
	if err != nil {
		return volumes.Volume{}, err
	}
	fileShareClient, err := login.NewFileShareClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return volumes.Volume{}, err
	}
	_, err = fileShareClient.Get(ctx, cs.aciContext.ResourceGroup, storageAccount, fileshareName, "")
	if err != nil { // Just checks if it exists
		return volumes.Volume{}, err
	}
	return toVolume(storageAccount, fileshareName), nil
}

func toVolume(storageAccountName string, fileShareName string) volumes.Volume {
	return volumes.Volume{
		ID:          volumeID(storageAccountName, fileShareName),
		Description: fmt.Sprintf("Fileshare %s in %s storage account", fileShareName, storageAccountName),
	}
}

func volumeID(storageAccount string, fileShareName string) string {
	return fmt.Sprintf("%s/%s", storageAccount, fileShareName)
}

func defaultStorageAccountParams(aciContext store.AciContext) storage.AccountCreateParameters {
	tags := map[string]*string{dockerVolumeTag: to.StringPtr(dockerVolumeTag)}
	return storage.AccountCreateParameters{
		Location: to.StringPtr(aciContext.Location),
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
		},
		Tags: tags,
	}
}

func getStorageAccountAndFileshare(volumeID string) (string, string, error) {
	tokens := strings.Split(volumeID, "/")
	if len(tokens) != 2 {
		return "", "", errors.New("invalid format for volume ID, expected storageaccount/fileshare")
	}
	return tokens[0], tokens[1], nil
}
