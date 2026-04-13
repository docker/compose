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
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
)

// executionState tracks the results of operations as they complete, allowing
// dependent operations to resolve service references.
type executionState struct {
	mu         sync.Mutex
	containers map[string]Containers // service name -> containers created/updated
}

func newExecutionState() *executionState {
	return &executionState{
		containers: make(map[string]Containers),
	}
}

// newExecutionStateFrom builds an executionState pre-populated with existing
// containers partitioned by service name. Used by run.go to resolve service
// references without the old convergence struct.
func newExecutionStateFrom(containers Containers) *executionState {
	es := newExecutionState()
	for _, c := range containers.filter(isNotOneOff) {
		service := c.Labels[api.ServiceLabel]
		es.containers[service] = append(es.containers[service], c)
	}
	return es
}

func (es *executionState) addContainer(serviceName string, ctr containerType.Summary) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.containers[serviceName] = append(es.containers[serviceName], ctr)
}

func (es *executionState) getContainers(serviceName string) Containers {
	es.mu.Lock()
	defer es.mu.Unlock()
	return slices.Clone(es.containers[serviceName])
}

// resolveServiceReferences replaces service references in a ServiceConfig with
// actual container IDs from the execution state. This mirrors the logic in
// convergence.resolveServiceReferences but uses executionState instead.
func (es *executionState) resolveServiceReferences(service *types.ServiceConfig) error {
	if err := es.resolveVolumeFrom(service); err != nil {
		return err
	}
	return es.resolveSharedNamespaces(service)
}

func (es *executionState) resolveVolumeFrom(service *types.ServiceConfig) error {
	for i, vol := range service.VolumesFrom {
		spec := strings.Split(vol, ":")
		if len(spec) == 0 {
			continue
		}
		if spec[0] == "container" {
			service.VolumesFrom[i] = spec[1]
			continue
		}
		name := spec[0]
		dependencies := es.getContainers(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share volume with service %s: container missing", name)
		}
		service.VolumesFrom[i] = dependencies.sorted()[0].ID
	}
	return nil
}

func (es *executionState) resolveSharedNamespaces(service *types.ServiceConfig) error {
	if name := getDependentServiceFromMode(service.NetworkMode); name != "" {
		dependencies := es.getContainers(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share network namespace with service %s: container missing", name)
		}
		service.NetworkMode = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	if name := getDependentServiceFromMode(service.Ipc); name != "" {
		dependencies := es.getContainers(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share IPC namespace with service %s: container missing", name)
		}
		service.Ipc = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	if name := getDependentServiceFromMode(service.Pid); name != "" {
		dependencies := es.getContainers(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share PID namespace with service %s: container missing", name)
		}
		service.Pid = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	return nil
}

// ExecutePlan executes a reconciliation plan using DAG traversal similar to
// graphTraversal.visit() in dependencies.go. Operations are executed
// concurrently, respecting dependency ordering.
func (s *composeService) ExecutePlan(ctx context.Context, project *types.Project, plan *ReconciliationPlan, observed *ObservedState) error {
	if plan.IsEmpty() {
		return nil
	}

	// Validate the plan has no dependency cycles before executing.
	// Without this check, a cycle would cause the executor to hang
	// indefinitely waiting for operations that can never be scheduled.
	if _, err := topologicalSort(plan); err != nil {
		return err
	}

	// Pre-populate execution state with existing containers so that
	// resolveServiceReferences can find containers for services not
	// included in the plan (e.g. --no-deps scenarios).
	state := newExecutionStateFrom(observed.allContainers())

	// Build dependency count map: number of unsatisfied deps per operation.
	// The consumer goroutine is single-threaded, so no mutex is needed for depCount.
	depCount := make(map[string]int, len(plan.Operations))
	for _, op := range plan.Operations {
		depCount[op.ID] = len(op.DependsOn)
	}

	expect := len(plan.Operations)
	eg, ctx := errgroup.WithContext(ctx)
	opCh := make(chan *Operation, expect)

	// sendDone sends a completed operation to the consumer goroutine,
	// respecting context cancellation to avoid blocking or panicking.
	sendDone := func(op *Operation) {
		select {
		case opCh <- op:
		case <-ctx.Done():
		}
	}

	// Consumer goroutine: waits for completed ops and enqueues newly-ready dependents
	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case doneOp := <-opCh:
				expect--
				if expect == 0 {
					return nil
				}

				// Decrement dep count for each dependent; schedule when ready
				for _, depID := range plan.Dependents[doneOp.ID] {
					depCount[depID]--
					if depCount[depID] == 0 {
						depOp := plan.Operations[depID]
						eg.Go(func() error {
							if err := s.executeOperation(ctx, project, depOp, state); err != nil {
								return err
							}
							sendDone(depOp)
							return nil
						})
					}
				}
			}
		}
	})

	// Launch root operations
	for _, op := range plan.Roots() {
		eg.Go(func() error {
			if err := s.executeOperation(ctx, project, op, state); err != nil {
				return err
			}
			sendDone(op)
			return nil
		})
	}

	return eg.Wait()
}

func (s *composeService) executeOperation(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	spanName := op.Type.String()
	opts := tracing.SpanOptions{}
	if op.ContainerOp != nil {
		opts = tracing.ServiceOptions(op.ContainerOp.Service)
	}
	ctx, span := otel.Tracer("").Start(ctx, spanName, opts.SpanStartOptions()...)
	defer span.End()
	span.SetAttributes(
		attribute.String("operation.id", op.ID),
		attribute.String("operation.resource", op.Resource),
		attribute.String("operation.reason", op.Reason),
	)

	err := s.dispatchOperation(ctx, project, op, state)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return err
}

func (s *composeService) dispatchOperation(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	switch op.Type {
	case OpCreateNetwork:
		return s.executePlanCreateNetwork(ctx, project, op)
	case OpRemoveNetwork:
		return s.executePlanRemoveNetwork(ctx, project, op)
	case OpDisconnectNetwork:
		return s.executePlanDisconnectNetwork(ctx, op)
	case OpConnectNetwork:
		return s.executePlanConnectNetwork(ctx, op)
	case OpCreateVolume:
		return s.executePlanCreateVolume(ctx, project, op)
	case OpRemoveVolume:
		return s.executePlanRemoveVolume(ctx, op)
	case OpCreateContainer:
		return s.executePlanCreateContainer(ctx, project, op, state)
	case OpStartContainer:
		return s.executePlanStartContainer(ctx, op)
	case OpStopContainer:
		return s.executePlanStopContainer(ctx, op)
	case OpRemoveContainer:
		return s.executePlanRemoveContainer(ctx, op)
	case OpRenameContainer:
		return s.executePlanRenameContainer(ctx, op)
	case OpRunPlugin:
		return s.executePlanRunPlugin(ctx, project, op)
	default:
		return fmt.Errorf("unknown operation type: %d", op.Type)
	}
}

func (s *composeService) executePlanCreateNetwork(ctx context.Context, project *types.Project, op *Operation) error {
	_, err := s.ensureNetwork(ctx, project, op.NetworkOp.NetworkKey, op.NetworkOp.Desired)
	return err
}

func (s *composeService) executePlanRemoveNetwork(ctx context.Context, project *types.Project, op *Operation) error {
	return s.removeNetwork(ctx, op.NetworkOp.NetworkKey, project.Name, op.NetworkOp.Existing.Name)
}

func (s *composeService) executePlanDisconnectNetwork(ctx context.Context, op *Operation) error {
	_, err := s.apiClient().NetworkDisconnect(ctx, op.ContainerNetworkOp.NetworkName, client.NetworkDisconnectOptions{
		Container: op.ContainerNetworkOp.ContainerID,
		Force:     true,
	})
	return err
}

func (s *composeService) executePlanConnectNetwork(ctx context.Context, op *Operation) error {
	_, err := s.apiClient().NetworkConnect(ctx, op.ContainerNetworkOp.NetworkName, client.NetworkConnectOptions{
		Container: op.ContainerNetworkOp.ContainerID,
	})
	return err
}

func (s *composeService) executePlanCreateVolume(ctx context.Context, project *types.Project, op *Operation) error {
	volume := *op.VolumeOp.Desired
	volume.CustomLabels = volume.CustomLabels.Add(api.VolumeLabel, op.VolumeOp.VolumeKey)
	volume.CustomLabels = volume.CustomLabels.Add(api.ProjectLabel, project.Name)
	volume.CustomLabels = volume.CustomLabels.Add(api.VersionLabel, api.ComposeVersion)
	_, err := s.ensureVolume(ctx, op.VolumeOp.VolumeKey, volume, project)
	return err
}

func (s *composeService) executePlanRemoveVolume(ctx context.Context, op *Operation) error {
	return s.removeVolume(ctx, op.VolumeOp.Existing.Name)
}

func (s *composeService) executePlanCreateContainer(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	service := op.ContainerOp.Service

	// Resolve service references using execution state, falling back to
	// pre-populated existing containers for --no-deps scenarios.
	if err := state.resolveServiceReferences(&service); err != nil {
		return err
	}

	eventName := "Container " + op.ContainerOp.ContainerName
	s.events.On(creatingEvent(eventName))

	labels := mergeLabels(service.Labels, service.CustomLabels)

	// When Existing is set, this is the "create" step of a recreate chain:
	// inherit from old container and add replace label.
	var inherited *containerType.Summary
	if op.ContainerOp.Existing != nil && op.ContainerOp.Inherit {
		inherited = op.ContainerOp.Existing
	}
	if op.ContainerOp.Existing != nil {
		replacedName := service.ContainerName
		if replacedName == "" {
			replacedName = service.Name + api.Separator + strconv.Itoa(op.ContainerOp.ContainerNumber)
		}
		labels = labels.Add(api.ContainerReplaceLabel, replacedName)
	}

	opts := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            labels,
	}

	ctr, err := s.createMobyContainer(ctx, project, service, op.ContainerOp.ContainerName, op.ContainerOp.ContainerNumber, inherited, opts)
	if err != nil {
		return err
	}
	s.events.On(createdEvent(eventName))

	state.addContainer(op.ServiceName, ctr)
	return nil
}

func (s *composeService) executePlanRenameContainer(ctx context.Context, op *Operation) error {
	eventName := "Container " + op.RenameOp.NewName
	s.events.On(newEvent(eventName, api.Working, "Recreate"))
	_, err := s.apiClient().ContainerRename(ctx, op.RenameOp.CurrentName, client.ContainerRenameOptions{
		NewName: op.RenameOp.NewName,
	})
	if err != nil {
		return err
	}
	s.events.On(newEvent(eventName, api.Done, "Recreated"))
	return nil
}

func (s *composeService) executePlanStartContainer(ctx context.Context, op *Operation) error {
	eventName := "Container " + op.ContainerOp.ContainerName
	var containerID string
	if op.ContainerOp.Existing != nil {
		containerID = op.ContainerOp.Existing.ID
	} else {
		// Container was just created/renamed; look it up by name
		res, err := s.apiClient().ContainerInspect(ctx, op.ContainerOp.ContainerName, client.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("cannot start container %s: %w", op.ContainerOp.ContainerName, err)
		}
		containerID = res.Container.ID
	}
	s.events.On(startingEvent(eventName))
	startMx.Lock()
	_, err := s.apiClient().ContainerStart(ctx, containerID, client.ContainerStartOptions{})
	startMx.Unlock()
	if err != nil {
		return err
	}
	s.events.On(startedEvent(eventName))
	return nil
}

func (s *composeService) executePlanStopContainer(ctx context.Context, op *Operation) error {
	var svc *types.ServiceConfig
	if op.ContainerOp.Service.Name != "" {
		s := op.ContainerOp.Service
		svc = &s
	}
	return s.stopContainer(ctx, svc, *op.ContainerOp.Existing, op.ContainerOp.Timeout, nil)
}

func (s *composeService) executePlanRemoveContainer(ctx context.Context, op *Operation) error {
	ctr := *op.ContainerOp.Existing
	eventName := getContainerProgressName(ctr)
	s.events.On(removingEvent(eventName))
	_, err := s.apiClient().ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil && !errdefs.IsNotFound(err) && !errdefs.IsConflict(err) {
		s.events.On(errorEvent(eventName, "Error while Removing"))
		return err
	}
	s.events.On(removedEvent(eventName))
	return nil
}

func (s *composeService) executePlanRunPlugin(ctx context.Context, project *types.Project, op *Operation) error {
	return s.runPlugin(ctx, project, op.PluginOp.Service, op.PluginOp.Action)
}

// DisplayPlan performs a topological sort of operations and displays them
// grouped by resource type.
func DisplayPlan(plan *ReconciliationPlan, w io.Writer) error {
	ops, err := topologicalSort(plan)
	if err != nil {
		return err
	}

	// Group operations by category
	var networkOps, volumeOps []*Operation
	serviceOps := make(map[string][]*Operation)

	for _, op := range ops {
		switch {
		case op.NetworkOp != nil:
			networkOps = append(networkOps, op)
		case op.ContainerNetworkOp != nil:
			networkOps = append(networkOps, op)
		case op.VolumeOp != nil:
			volumeOps = append(volumeOps, op)
		case op.ContainerOp != nil:
			serviceOps[op.ServiceName] = append(serviceOps[op.ServiceName], op)
		case op.RenameOp != nil:
			serviceOps[op.ServiceName] = append(serviceOps[op.ServiceName], op)
		case op.PluginOp != nil:
			serviceOps[op.ServiceName] = append(serviceOps[op.ServiceName], op)
		}
	}

	if err := displayOpsSection(w, "Networks:", "  ", networkOps); err != nil {
		return err
	}
	if err := displayOpsSection(w, "Volumes:", "  ", volumeOps); err != nil {
		return err
	}
	return displayServiceOps(w, serviceOps)
}

func displayOpsSection(w io.Writer, header, indent string, ops []*Operation) error {
	if len(ops) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	for _, op := range ops {
		if _, err := fmt.Fprintf(w, "%s[%-10s] %-20s reason: %s\n", indent, opVerb(op.Type), op.Resource, op.Reason); err != nil {
			return err
		}
	}
	return nil
}

func displayServiceOps(w io.Writer, serviceOps map[string][]*Operation) error {
	if len(serviceOps) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Services:"); err != nil {
		return err
	}

	// Sort service names for stable output
	serviceNames := make([]string, 0, len(serviceOps))
	for name := range serviceOps {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, svcName := range serviceNames {
		if _, err := fmt.Fprintf(w, "  %s:\n", svcName); err != nil {
			return err
		}
		for _, op := range serviceOps[svcName] {
			if _, err := fmt.Fprintf(w, "    [%-10s] %-20s reason: %s\n", opVerb(op.Type), op.Resource, op.Reason); err != nil {
				return err
			}
		}
	}
	return nil
}

// opVerb returns a short action verb for display purposes.
func opVerb(t OperationType) string {
	switch t {
	case OpCreateNetwork, OpCreateVolume, OpCreateContainer:
		return "create"
	case OpRenameContainer:
		return "rename"
	case OpRemoveNetwork, OpRemoveVolume, OpRemoveContainer:
		return "remove"
	case OpDisconnectNetwork:
		return "disconnect"
	case OpConnectNetwork:
		return "connect"
	case OpStartContainer:
		return "start"
	case OpStopContainer:
		return "stop"
	case OpRunPlugin:
		return "plugin"
	default:
		return "unknown"
	}
}

// topologicalSort returns operations in dependency order using Kahn's algorithm.
// Returns an error if the dependency graph contains a cycle.
func topologicalSort(plan *ReconciliationPlan) ([]*Operation, error) {
	inDegree := make(map[string]int, len(plan.Operations))
	for _, op := range plan.Operations {
		inDegree[op.ID] = len(op.DependsOn)
	}

	// Start with nodes that have no dependencies
	var queue []string
	for _, op := range plan.Operations {
		if inDegree[op.ID] == 0 {
			queue = append(queue, op.ID)
		}
	}
	sort.Strings(queue) // deterministic ordering

	var sorted []*Operation
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, plan.Operations[id])

		var next []string
		for _, depID := range plan.Dependents[id] {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				next = append(next, depID)
			}
		}
		sort.Strings(next)
		queue = append(queue, next...)
	}

	if len(sorted) != len(plan.Operations) {
		var cycled []string
		for id, degree := range inDegree {
			if degree > 0 {
				cycled = append(cycled, id)
			}
		}
		sort.Strings(cycled)
		return nil, fmt.Errorf("dependency cycle detected involving operations: %v", cycled)
	}

	return sorted, nil
}
