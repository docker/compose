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

	"github.com/hashicorp/go-multierror"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/context/store"
)

type aciResourceService struct {
	aciContext store.AciContext
}

func (cs *aciResourceService) Prune(ctx context.Context, request resources.PruneRequest) ([]string, error) {
	res, err := getACIContainerGroups(ctx, cs.aciContext.SubscriptionID, cs.aciContext.ResourceGroup)
	if err != nil {
		return nil, err
	}
	multierr := &multierror.Error{}
	deleted := []string{}
	for _, containerGroup := range res {
		if !request.Force && convert.GetGroupStatus(containerGroup) == "Node "+convert.StatusRunning {
			continue
		}

		if !request.DryRun {
			_, err := deleteACIContainerGroup(ctx, cs.aciContext, *containerGroup.Name)
			multierr = multierror.Append(multierr, err)
		}
		if err == nil {
			deleted = append(deleted, *containerGroup.Name)
		}
	}
	return deleted, multierr.ErrorOrNil()
}
