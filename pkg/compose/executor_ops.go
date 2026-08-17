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
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// --- Network operations ---

func (exec *planExecutor) execCreateNetwork(ctx context.Context, op Operation) error {
	return exec.compose.createNetwork(ctx, op.Network)
}

func (exec *planExecutor) execRemoveNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkRemove(ctx, op.Name, client.NetworkRemoveOptions{})
	// A best-effort removal (old network on a rename) tolerates the network
	// still being in use — Docker reports that as a conflict. Any other error
	// (transport failure, Moby API error, ...) is still propagated.
	if err != nil && op.BestEffort && errdefs.IsConflict(err) {
		logrus.Warnf("network %s is still in use and was left in place; remove it manually once no container is attached", op.Name)
		return nil
	}
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

// execStartContainer starts a container. When the operation carries a Service
// (start-phase nodes), it performs the full service start: secret/config
// injection before ContainerStart, post_start hooks after — mirroring
// startServiceContainer. Bare operations (paused/dead restarts) keep the
// plain ContainerStart.
func (exec *planExecutor) execStartContainer(ctx context.Context, op Operation) error {
	id, name := exec.resolveContainer(op)
	if id == "" {
		return fmt.Errorf("no container to start for %s", op.ResourceID)
	}

	if op.Service != nil {
		if err := exec.compose.injectSecrets(ctx, exec.project, *op.Service, id); err != nil {
			return err
		}
		if err := exec.compose.injectConfigs(ctx, exec.project, *op.Service, id); err != nil {
			return err
		}
	}

	startMx.Lock()
	_, err := exec.compose.apiClient().ContainerStart(ctx, id, client.ContainerStartOptions{})
	startMx.Unlock()
	if err != nil {
		return err
	}

	if op.Service != nil {
		ctr := exec.containerSummary(op, id, name)
		for _, hook := range op.Service.PostStart {
			if err := exec.compose.runHook(ctx, ctr, *op.Service, hook, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveContainer returns the ID and display name of the container an
// operation targets: the observed container when set, otherwise the result of
// the create node the operation references.
func (exec *planExecutor) resolveContainer(op Operation) (string, string) {
	if op.Container != nil {
		return op.Container.ID, getCanonicalContainerName(*op.Container)
	}
	res := exec.pctx.get(op.CreateNodeID)
	name := op.Name
	if name == "" {
		name = res.ContainerName
	}
	return res.ContainerID, name
}

// containerSummary rebuilds a minimal container.Summary for helpers (hooks)
// that need one, preferring the live view populated by earlier create nodes.
func (exec *planExecutor) containerSummary(op Operation, id, name string) container.Summary {
	if op.Container != nil {
		return *op.Container
	}
	exec.containersMu.Lock()
	defer exec.containersMu.Unlock()
	if op.Service != nil {
		for _, c := range exec.containersByService[op.Service.Name] {
			if c.ID == id {
				return c
			}
		}
	}
	return container.Summary{ID: id, Names: []string{"/" + name}}
}

// execWaitCondition polls the dependency service named by the operation until
// it satisfies the declared depends_on condition, the deadline expires, or
// the context ends. It is the plan-side equivalent of waitDependencies for a
// single (dependent, dependency) edge; required: false edges are planned as
// Optional nodes, so their failure is reported as a skip by the walker.
func (exec *planExecutor) execWaitCondition(ctx context.Context, op Operation) error {
	if op.Timeout != nil {
		withTimeout, cancel := context.WithTimeout(ctx, *op.Timeout)
		defer cancel()
		ctx = withTimeout
	}

	exec.containersMu.Lock()
	waitingFor := exec.containersByService[op.Name].filter(isNotOneOff)
	exec.containersMu.Unlock()
	if len(waitingFor) == 0 {
		return fmt.Errorf("missing dependency %s", op.Name)
	}
	exec.compose.events.On(containerEvents(waitingFor, waiting)...)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for dependencies")
			}
			return ctx.Err()
		}
		satisfied, err := exec.checkWaitCondition(ctx, op, waitingFor)
		if err != nil {
			return err
		}
		if satisfied {
			return nil
		}
	}
}

// checkWaitCondition performs one probe of a WaitCondition operation,
// reporting whether the dependency satisfies the declared condition.
func (exec *planExecutor) checkWaitCondition(ctx context.Context, op Operation, waitingFor Containers) (bool, error) {
	s := exec.compose
	switch op.Condition {
	case ServiceConditionRunningOrHealthy, types.ServiceConditionHealthy:
		fallbackRunning := op.Condition == ServiceConditionRunningOrHealthy
		ok, err := s.isServiceHealthy(ctx, waitingFor, fallbackRunning)
		if err != nil {
			return false, fmt.Errorf("dependency failed to start: %w", err)
		}
		if ok {
			s.events.On(containerEvents(waitingFor, healthy)...)
		}
		return ok, nil
	case types.ServiceConditionCompletedSuccessfully:
		done, code, err := s.isServiceCompleted(ctx, waitingFor)
		if err != nil {
			return false, err
		}
		if !done {
			return false, nil
		}
		if code != 0 {
			return false, fmt.Errorf("service %q didn't complete successfully: exit %d", op.Name, code)
		}
		s.events.On(containerEvents(waitingFor, exited)...)
		return true, nil
	default:
		logrus.Warnf("unsupported depends_on condition: %s", op.Condition)
		return true, nil
	}
}

// execRunPreStart runs the service's pre_start hooks on the replica the plan
// selected (once per service, before any of its containers start).
func (exec *planExecutor) execRunPreStart(ctx context.Context, op Operation) error {
	// The plan scheduled this node because no replica was running at
	// observation time. Re-check against the daemon at execution time: if a
	// replica started in the meantime (observe-to-execute drift), the
	// once-per-service rule says the hooks must not run again.
	running, err := exec.compose.getContainers(ctx, exec.project.Name, oneOffExclude, false, op.Service.Name)
	if err != nil {
		return err
	}
	if len(running) > 0 {
		logrus.Debugf("skipping pre_start hooks of service %s: a replica is already running", op.Service.Name)
		return nil
	}

	id, name := exec.resolveContainer(op)
	if id == "" {
		return fmt.Errorf("no container to run pre_start hooks for %s", op.ResourceID)
	}
	ctr := exec.containerSummary(op, id, name)
	return exec.compose.runPreStart(ctx, exec.project, *op.Service, ctr, nil)
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
