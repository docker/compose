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

	"github.com/hashicorp/go-multierror"

	"github.com/docker/compose-cli/aci/convert"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/resources"
)

type aciResourceService struct {
	aciContext store.AciContext
}

func (cs *aciResourceService) Prune(ctx context.Context, request resources.PruneRequest) (resources.PruneResult, error) {
	res, err := getACIContainerGroups(ctx, cs.aciContext.SubscriptionID, cs.aciContext.ResourceGroup)
	result := resources.PruneResult{}
	if err != nil {
		return result, err
	}
	multierr := &multierror.Error{}
	deleted := []string{}
	cpus := 0.
	mem := 0.

	for _, containerGroup := range res {
		if !request.Force && convert.GetGroupStatus(containerGroup) == "Node "+convert.StatusRunning {
			continue
		}

		for _, container := range *containerGroup.Containers {
			hostConfig := convert.ToHostConfig(container, containerGroup)
			cpus += hostConfig.CPUReservation
			mem += convert.BytesToGB(float64(hostConfig.MemoryReservation))
		}

		if !request.DryRun {
			_, err := deleteACIContainerGroup(ctx, cs.aciContext, *containerGroup.Name)
			multierr = multierror.Append(multierr, err)
		}

		deleted = append(deleted, *containerGroup.Name)
	}
	result.DeletedIDs = deleted
	result.Summary = fmt.Sprintf("Total CPUs reclaimed: %.2f, total memory reclaimed: %.2f GB", cpus, mem)
	return result, multierr.ErrorOrNil()
}
