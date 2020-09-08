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

package aci

import (
	"context"
	"fmt"

	"github.com/docker/compose-cli/progress"

	"github.com/Azure/go-autorest/autorest/to"

	"github.com/docker/compose-cli/aci/login"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"

	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/errdefs"

	"github.com/pkg/errors"

	"github.com/docker/compose-cli/context/store"
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
				fileShares = append(fileShares, toVolume(account, *fileShare.Name))
			}
			if err := fileSharePage.NextWithContext(ctx); err != nil {
				return nil, err
			}
		}
	}
	return fileShares, nil
}

//VolumeCreateOptions options to create a new ACI volume
type VolumeCreateOptions struct {
	Account   string
	Fileshare string
}

//VolumeDeleteOptions options to create a new ACI volume
type VolumeDeleteOptions struct {
	Account       string
	Fileshare     string
	DeleteAccount bool
}

func (cs *aciVolumeService) Create(ctx context.Context, options interface{}) (volumes.Volume, error) {
	opts, ok := options.(VolumeCreateOptions)
	if !ok {
		return volumes.Volume{}, errors.New("Could not read azure LoginParams struct from generic parameter")
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
	} else {
		if account.StatusCode != 404 {
			return volumes.Volume{}, err
		}
		//TODO confirm storage account creation
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
		err = future.WaitForCompletionRef(ctx, accountClient.Client)
		if err != nil {
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
	w.Event(event(opts.Fileshare, progress.Working, "Creating"))
	fileShareClient, err := login.NewFileShareClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return volumes.Volume{}, err
	}

	fileShare, err := fileShareClient.Get(ctx, cs.aciContext.ResourceGroup, *account.Name, opts.Fileshare, "")
	if err == nil {
		w.Event(errorEvent(opts.Fileshare))
		return volumes.Volume{}, errors.Wrapf(errdefs.ErrAlreadyExists, "Azure fileshare %q already exists", opts.Fileshare)
	}
	if fileShare.StatusCode != 404 {
		w.Event(errorEvent(opts.Fileshare))
		return volumes.Volume{}, err
	}
	//TODO tag fileshare
	fileShare, err = fileShareClient.Create(ctx, cs.aciContext.ResourceGroup, *account.Name, opts.Fileshare, storage.FileShare{})
	if err != nil {
		w.Event(errorEvent(opts.Fileshare))
		return volumes.Volume{}, err
	}
	w.Event(event(opts.Fileshare, progress.Done, "Created"))
	return toVolume(account, *fileShare.Name), nil
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

func (cs *aciVolumeService) Delete(ctx context.Context, options interface{}) error {
	opts, ok := options.(VolumeDeleteOptions)
	if !ok {
		return errors.New("Could not read azure VolumeDeleteOptions struct from generic parameter")
	}
	if opts.DeleteAccount {
		//TODO check if there are other fileshares on this account
		//TODO flag account and only delete ours?
		//TODO error when not found
		storageAccountsClient, err := login.NewStorageAccountsClient(cs.aciContext.SubscriptionID)
		if err != nil {
			return err
		}

		_, err = storageAccountsClient.Delete(ctx, cs.aciContext.ResourceGroup, opts.Account)
		return err
	}

	fileShareClient, err := login.NewFileShareClient(cs.aciContext.SubscriptionID)
	if err != nil {
		return err
	}

	_, err = fileShareClient.Delete(ctx, cs.aciContext.ResourceGroup, opts.Account, opts.Fileshare)
	return err
}

func toVolume(account storage.Account, fileShareName string) volumes.Volume {
	return volumes.Volume{
		ID:          fmt.Sprintf("%s@%s", *account.Name, fileShareName),
		Name:        fileShareName,
		Description: fmt.Sprintf("Fileshare %s in %s storage account", fileShareName, *account.Name),
	}
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
