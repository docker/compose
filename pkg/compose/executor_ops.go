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

	options := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            labels,
	}
	ctr, err := exec.compose.createMobyContainer(ctx, exec.project, service, op.Name, op.Number, op.Inherited, options)
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

// execStartContainer starts a container. A bare operation (no Service —
// today only the create phase's exceptional-state restart of a paused/dead
// container) is a plain ContainerStart. A start-phase operation (Service set)
// performs the full service start — secret/config injection right before
// ContainerStart — mirroring startServiceContainer; its target resolves
// either from the observed container or, for a replica the plan itself
// creates, from the CreateContainer node's result (the same mechanism
// OpRenameContainer already uses).
func (exec *planExecutor) execStartContainer(ctx context.Context, op Operation) error {
	if op.Service == nil {
		startMx.Lock()
		defer startMx.Unlock()
		_, err := exec.compose.apiClient().ContainerStart(ctx, op.Container.ID, client.ContainerStartOptions{})
		return err
	}

	// A dependency's done-channel closes (unblocking this node) before
	// errgroup cancels ctx on that same dependency's error — cancel() only
	// runs after the failing goroutine returns, close(done[...]) runs inside
	// it. This check narrows that window (same guard as
	// execCreateHookContainer) but cannot close it: a preceding node of this
	// replica's chain (a wait, pre_start) can fail for a genuine reason a
	// moment before this goroutine observes ctx.Err(), still nil, and starts
	// a container whose pre_start hook just failed. The structural fix —
	// carrying each node's success/failure through what dependents wait on,
	// instead of inferring it from ctx.Err() — is tracked as its own brick of
	// the executor lot (#14081, see the epic's "failed-dependency semantics"
	// comment); every per-operation ctx.Err() guard here, including this one,
	// is an interim narrowing, not the fix.
	if err := ctx.Err(); err != nil {
		return err
	}

	id, err := exec.resolveContainerID(op)
	if err != nil {
		return err
	}
	if err := exec.compose.injectSecrets(ctx, exec.project, *op.Service, id); err != nil {
		return err
	}
	if err := exec.compose.injectConfigs(ctx, exec.project, *op.Service, id); err != nil {
		return err
	}

	startMx.Lock()
	_, err = exec.compose.apiClient().ContainerStart(ctx, id, client.ContainerStartOptions{})
	startMx.Unlock()
	return err
}

// resolveContainerID returns the ID of the container an operation targets:
// the observed container when the reconciler already had one, otherwise the
// result of the CreateContainer node it references.
func (exec *planExecutor) resolveContainerID(op Operation) (string, error) {
	if op.Container != nil {
		return op.Container.ID, nil
	}
	res := exec.pctx.get(op.CreateNodeID)
	if res.ContainerID == "" {
		return "", fmt.Errorf("internal: no materialized container for %s", op.ResourceID)
	}
	return res.ContainerID, nil
}

// resolveContainerSummary is resolveContainerID plus the rest of the
// container.Summary that hook execution (runHook) needs — preferring the
// observed container, falling back to the live view populated by the create
// node execCreateContainer already ran (a dependency of every start-phase
// node targeting that replica).
func (exec *planExecutor) resolveContainerSummary(op Operation) (container.Summary, error) {
	if op.Container != nil {
		return *op.Container, nil
	}
	id, err := exec.resolveContainerID(op)
	if err != nil {
		return container.Summary{}, err
	}
	exec.containersMu.Lock()
	defer exec.containersMu.Unlock()
	for _, c := range exec.containersByService[op.Service.Name] {
		if c.ID == id {
			return c, nil
		}
	}
	return container.Summary{ID: id, Names: []string{"/" + exec.pctx.get(op.CreateNodeID).ContainerName}}, nil
}

// execWaitCondition polls the dependency service named by the operation
// until it satisfies the declared depends_on condition or the context ends —
// the plan-side equivalent of one waitDependencies edge, reusing the exact
// same polling primitive (waitDependency) and per-condition checks the
// imperative engine uses, so events and error messages cannot drift between
// the two. required: false dependencies are marked BestEffort by the
// reconciler: a missing dependency or a failed/timed-out condition is then a
// warning, not a plan failure — matching waitDependencies' own
// optional-dependency handling.
//
// Unlike waitDependencies, this applies no deadline of its own: nothing
// produces one yet (no ReconcileOptions field feeds a per-wait timeout the
// way api.CreateOptions.WaitTimeout does today). A future caller needing that
// — e.g. `up --wait` once it runs on the plan — wraps ctx before executing
// the plan, or adds a Timeout to the operation for execWaitCondition to wrap
// here.
func (exec *planExecutor) execWaitCondition(ctx context.Context, op Operation) error {
	s := exec.compose
	exec.containersMu.Lock()
	waitingFor := exec.containersByService[op.Name].filter(isNotOneOff, isNotHookContainer)
	exec.containersMu.Unlock()

	config := types.ServiceDependency{Condition: op.Condition, Required: !op.BestEffort}

	if len(waitingFor) == 0 {
		if config.Required {
			return fmt.Errorf("missing dependency %s", op.Name)
		}
		logrus.Warnf("missing dependency %s", op.Name)
		return nil
	}

	s.events.On(containerEvents(waitingFor, waiting)...)
	// The wait node is shared across every dependent awaiting the same
	// (dependency, condition) pair (see waitConditionNode), so no single
	// requester name would be accurate; op.ResourceID identifies the wait
	// itself instead. waitDependency only ever reads this for one
	// practically unreachable log line (an unsupported depends_on condition,
	// filtered out before a plan is ever built).
	return s.waitDependency(ctx, op.ResourceID, op.Name, config, waitingFor)
}

// execRunPreStart runs the service's pre_start hooks against the runner
// containers the create phase prepared (see execCreateHookContainer). The
// "once per service, no replica running at observation" rule is a plan-time
// decision; this re-checks the daemon first so a replica started in the
// window between observation and execution — the drift the plan design
// accepts — skips a second, redundant run of the hooks rather than erroring
// on runners already consumed.
func (exec *planExecutor) execRunPreStart(ctx context.Context, op Operation) error {
	// Same interim narrowing as execStartContainer's guard (see its
	// comment), not a full fix: this node hangs off the replica's create
	// node, whose done-channel closes before errgroup's ctx is canceled when
	// that create fails for a genuine reason, so a narrow window remains
	// where pre_start hooks could still run against the runner containers.
	if err := ctx.Err(); err != nil {
		return err
	}

	running, err := exec.compose.getContainers(ctx, exec.project.Name, oneOffExclude, false, op.Service.Name)
	if err != nil {
		return err
	}
	if len(running) > 0 {
		logrus.Debugf("skipping pre_start hooks of service %s: a replica is already running", op.Service.Name)
		return nil
	}
	return exec.compose.runPreStart(ctx, exec.project, *op.Service, exec.listener)
}

// execRunPostStart runs the service's post_start hooks against the replica
// the start chain just brought up.
func (exec *planExecutor) execRunPostStart(ctx context.Context, op Operation) error {
	// Same interim narrowing as execStartContainer's guard above, not a full
	// fix (see its comment): a failed StartContainer's done-channel closes
	// before errgroup's ctx is canceled, so a narrow window remains where
	// this could still run post_start against a container whose start just
	// failed for a genuine reason.
	if err := ctx.Err(); err != nil {
		return err
	}

	ctr, err := exec.resolveContainerSummary(op)
	if err != nil {
		return err
	}
	for _, hook := range op.Service.PostStart {
		if err := exec.compose.runHook(ctx, ctr, *op.Service, hook, exec.listener); err != nil {
			return err
		}
	}
	return nil
}

func (exec *planExecutor) execStopContainer(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().ContainerStop(ctx, op.Container.ID, client.ContainerStopOptions{
		Timeout: utils.DurationSecondToInt(op.Timeout),
	})
	return err
}

func (exec *planExecutor) execRemoveContainer(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().ContainerRemove(ctx, op.Container.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: op.RemoveVolumes})
	if err != nil {
		if op.BestEffort {
			// warn-only removal (stale pre_start hook runner): the container
			// stays visible to the operator, the plan carries on — and the
			// live view below keeps it, since it was not removed
			logrus.Warnf("failed to remove %s: %v", op.ResourceID, err)
			return nil
		}
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

// execCreateHookContainer creates the runner container for one pre_start hook.
// The target replica — whose volumes the hook shares via VolumesFrom — is
// resolved from the live view at execution time: the node depends on every
// container operation of its service, so the view is final here, and the
// lowest-numbered replica matches the one the start phase hands the hooks.
func (exec *planExecutor) execCreateHookContainer(ctx context.Context, node *PlanNode) error {
	// A dependency's done-channel also closes on failure: when a replica
	// create failed and canceled the run, bail out on the cancellation
	// instead of reporting the empty live view as an internal error.
	if err := ctx.Err(); err != nil {
		return err
	}
	op := node.Operation
	service := *op.Service
	exec.containersMu.Lock()
	replicas := slices.Clone(exec.containersByService[service.Name])
	exec.containersMu.Unlock()
	if len(replicas) == 0 {
		return fmt.Errorf("internal: no %q container to attach pre_start hook %d to", service.Name, op.HookIndex)
	}
	target := lowestNumberedContainer(replicas)
	created, err := exec.compose.createPreStartContainer(ctx, exec.project, service, target, op.HookIndex, op.Name)
	if err != nil {
		return err
	}
	// Recorded for uniformity with execCreateContainer — every create
	// operation publishes its result. No plan operation consumes a hook
	// runner's entry today: the start phase rediscovers runners through
	// listPreStartRunners, by design.
	exec.pctx.set(node.ID, operationResult{
		ContainerID:   created.ID,
		ContainerName: op.Name,
	})
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
