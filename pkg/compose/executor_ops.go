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

package compose

import (
	"context"
	"fmt"
	"slices"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// --- Network operations ---

func (exec *planExecutor) execCreateNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.ensureNetwork(ctx, exec.project, op.Name, op.Network)
	return err
}

func (exec *planExecutor) execRemoveNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkRemove(ctx, op.Name, client.NetworkRemoveOptions{})
	return err
}

func (exec *planExecutor) execDisconnectNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkDisconnect(ctx, op.Name, client.NetworkDisconnectOptions{
		Container: op.Container.ID,
		Force:     true,
	})
	return err
}

func (exec *planExecutor) execConnectNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkConnect(ctx, op.Name, client.NetworkConnectOptions{
		Container: op.Container.ID,
	})
	return err
}

// --- Volume operations ---

func (exec *planExecutor) execCreateVolume(ctx context.Context, op Operation) error {
	return exec.compose.createVolume(ctx, *op.Volume)
}

func (exec *planExecutor) execRemoveVolume(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().VolumeRemove(ctx, op.Name, client.VolumeRemoveOptions{Force: true})
	return err
}

// --- Container operations ---

func (exec *planExecutor) execCreateContainer(ctx context.Context, node *PlanNode) error {
	op := node.Operation
	service := *op.Service
	// Detach VolumesFrom from the source slice: resolveServiceReferences mutates
	// entries in place, and the shallow struct copy still shares the backing array.
	service.VolumesFrom = slices.Clone(op.Service.VolumesFrom)

	// Resolve service references (network_mode, ipc, pid, volumes_from) to
	// actual container IDs from the in-memory view, which already includes
	// any containers created by earlier plan nodes.
	exec.containersMu.Lock()
	err := resolveServiceReferences(&service, exec.containersByService)
	exec.containersMu.Unlock()
	if err != nil {
		return err
	}

	labels := mergeLabels(service.Labels, service.CustomLabels)
	if op.Inherited != nil {
		// This is a recreate: add the replace label
		replacedName := op.Service.ContainerName
		if replacedName == "" {
			replacedName = fmt.Sprintf("%s%s%d", op.Service.Name, api.Separator, op.Number)
		}
		labels = labels.Add(api.ContainerReplaceLabel, replacedName)
	}

	opts := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            labels,
	}
	ctr, err := exec.compose.createMobyContainer(ctx, exec.project, service, op.Name, op.Number, op.Inherited, opts)
	if err != nil {
		return err
	}

	exec.pctx.set(node.ID, operationResult{
		ContainerID:   ctr.ID,
		ContainerName: op.Name,
	})

	// Make the new container visible to subsequent execCreateContainer calls
	// that resolve service references against op.Service.Name.
	exec.containersMu.Lock()
	exec.containersByService[op.Service.Name] = append(exec.containersByService[op.Service.Name], ctr)
	exec.containersMu.Unlock()
	return nil
}

func (exec *planExecutor) execStartContainer(ctx context.Context, op Operation) error {
	startMx.Lock()
	defer startMx.Unlock()
	_, err := exec.compose.apiClient().ContainerStart(ctx, op.Container.ID, client.ContainerStartOptions{})
	return err
}

func (exec *planExecutor) execStopContainer(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().ContainerStop(ctx, op.Container.ID, client.ContainerStopOptions{
		Timeout: utils.DurationSecondToInt(op.Timeout),
	})
	return err
}

func (exec *planExecutor) execRemoveContainer(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().ContainerRemove(ctx, op.Container.ID, client.ContainerRemoveOptions{Force: true})
	if err != nil {
		return err
	}
	// Why: a dependent service's create may resolve `network_mode: service:X`
	// (or volumes_from / ipc / pid) against the live view. Containers.sorted()
	// orders by canonical name; without this drop, a just-removed container
	// can still win the lookup and the dependent receives a container:<id>
	// reference that no longer exists in the daemon.
	svcName := op.Container.Labels[api.ServiceLabel]
	exec.containersMu.Lock()
	exec.containersByService[svcName] = slices.DeleteFunc(
		exec.containersByService[svcName],
		func(c container.Summary) bool { return c.ID == op.Container.ID },
	)
	exec.containersMu.Unlock()
	return nil
}

func (exec *planExecutor) execRenameContainer(ctx context.Context, node *PlanNode) error {
	op := node.Operation
	if op.CreateNodeID == 0 {
		return fmt.Errorf("internal: rename node #%d missing CreateNodeID", node.ID)
	}
	createdID := exec.pctx.get(op.CreateNodeID).ContainerID
	if createdID == "" {
		return fmt.Errorf("internal: rename node #%d: create node #%d returned empty ID", node.ID, op.CreateNodeID)
	}
	_, err := exec.compose.apiClient().ContainerRename(ctx, createdID, client.ContainerRenameOptions{
		NewName: op.Name,
	})
	return err
}
