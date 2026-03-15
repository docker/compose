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
	"sort"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	containerType "github.com/moby/moby/api/types/container"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

// executionState tracks the results of operations as they complete, allowing
// dependent operations to resolve service references.
type executionState struct {
	mu         sync.Mutex
	containers map[string]Containers // service name -> containers created/updated
	networks   map[string]string     // network key -> ID
	volumes    map[string]string     // volume key -> ID
}

func newExecutionState() *executionState {
	return &executionState{
		containers: make(map[string]Containers),
		networks:   make(map[string]string),
		volumes:    make(map[string]string),
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
	return es.containers[serviceName]
}

func (es *executionState) setNetworkID(key, id string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.networks[key] = id
}

func (es *executionState) setVolumeID(key, id string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.volumes[key] = id
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
func (s *composeService) ExecutePlan(ctx context.Context, project *types.Project, plan *ReconciliationPlan) error {
	if plan.IsEmpty() {
		return nil
	}

	state := newExecutionState()

	// Build dependency count map
	depCount := make(map[string]int, len(plan.Operations))
	for _, op := range plan.Operations {
		depCount[op.ID] = len(op.DependsOn)
	}

	// Track completed operations
	var completedMu sync.Mutex
	completed := make(map[string]struct{})

	markCompleted := func(id string) {
		completedMu.Lock()
		defer completedMu.Unlock()
		completed[id] = struct{}{}
	}

	allDepsCompleted := func(op *Operation) bool {
		completedMu.Lock()
		defer completedMu.Unlock()
		for _, dep := range op.DependsOn {
			if _, done := completed[dep]; !done {
				return false
			}
		}
		return true
	}

	expect := len(plan.Operations)
	eg, ctx := errgroup.WithContext(ctx)
	opCh := make(chan *Operation, expect)
	defer close(opCh)

	// Consumer goroutine: waits for completed ops and enqueues newly-ready dependents
	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case doneOp := <-opCh:
				markCompleted(doneOp.ID)
				expect--
				if expect == 0 {
					return nil
				}

				// Check which dependents are now ready
				for _, depID := range plan.Dependents[doneOp.ID] {
					depOp := plan.Operations[depID]
					if allDepsCompleted(depOp) {
						eg.Go(func() error {
							err := s.executeOperation(ctx, project, depOp, state)
							opCh <- depOp
							return err
						})
					}
				}
			}
		}
	})

	// Launch root operations
	for _, op := range plan.Roots() {
		eg.Go(func() error {
			err := s.executeOperation(ctx, project, op, state)
			opCh <- op
			return err
		})
	}

	return eg.Wait()
}

func (s *composeService) executeOperation(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	switch op.Type {
	case OpCreateNetwork:
		return s.executePlanCreateNetwork(ctx, project, op, state)
	case OpRecreateNetwork:
		return s.executePlanCreateNetwork(ctx, project, op, state)
	case OpRemoveNetwork:
		return s.executePlanRemoveNetwork(ctx, project, op)
	case OpCreateVolume:
		return s.executePlanCreateVolume(ctx, project, op, state)
	case OpRecreateVolume:
		return s.executePlanCreateVolume(ctx, project, op, state)
	case OpRemoveVolume:
		return s.executePlanRemoveVolume(ctx, op)
	case OpCreateContainer:
		return s.executePlanCreateContainer(ctx, project, op, state)
	case OpRecreateContainer:
		return s.executePlanRecreateContainer(ctx, project, op, state)
	case OpStartContainer:
		return s.executePlanStartContainer(ctx, op)
	case OpStopContainer:
		return s.executePlanStopContainer(ctx, op)
	case OpRemoveContainer:
		return s.executePlanRemoveContainer(ctx, op)
	case OpRunPlugin:
		return s.executePlanRunPlugin(ctx, project, op)
	default:
		return fmt.Errorf("unknown operation type: %d", op.Type)
	}
}

func (s *composeService) executePlanCreateNetwork(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	id, err := s.ensureNetwork(ctx, project, op.NetworkOp.NetworkKey, op.NetworkOp.Desired)
	if err != nil {
		return err
	}
	state.setNetworkID(op.NetworkOp.NetworkKey, id)
	return nil
}

func (s *composeService) executePlanRemoveNetwork(ctx context.Context, project *types.Project, op *Operation) error {
	return s.removeNetwork(ctx, op.NetworkOp.NetworkKey, project.Name, op.NetworkOp.Existing.Name)
}

func (s *composeService) executePlanCreateVolume(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	volume := *op.VolumeOp.Desired
	volume.CustomLabels = volume.CustomLabels.Add(api.VolumeLabel, op.VolumeOp.VolumeKey)
	volume.CustomLabels = volume.CustomLabels.Add(api.ProjectLabel, project.Name)
	volume.CustomLabels = volume.CustomLabels.Add(api.VersionLabel, api.ComposeVersion)
	id, err := s.ensureVolume(ctx, op.VolumeOp.VolumeKey, volume, project)
	if err != nil {
		return err
	}
	state.setVolumeID(op.VolumeOp.VolumeKey, id)
	return nil
}

func (s *composeService) executePlanRemoveVolume(ctx context.Context, op *Operation) error {
	return s.removeVolume(ctx, op.VolumeOp.Existing.Name)
}

func (s *composeService) executePlanCreateContainer(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	service := op.ContainerOp.Service

	// Resolve service references using execution state
	if err := state.resolveServiceReferences(&service); err != nil {
		return err
	}

	opts := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            mergeLabels(service.Labels, service.CustomLabels),
	}

	ctr, err := s.createContainer(ctx, project, service, op.ContainerOp.ContainerName, op.ContainerOp.ContainerNumber, opts)
	if err != nil {
		return err
	}

	state.addContainer(op.ServiceName, ctr)
	return nil
}

func (s *composeService) executePlanRecreateContainer(ctx context.Context, project *types.Project, op *Operation, state *executionState) error {
	service := op.ContainerOp.Service

	// Resolve service references using execution state
	if err := state.resolveServiceReferences(&service); err != nil {
		return err
	}

	existing := op.ContainerOp.Existing
	created, err := s.recreateContainer(ctx, project, service, *existing, op.ContainerOp.Inherit, op.ContainerOp.Timeout)
	if err != nil {
		return err
	}

	state.addContainer(op.ServiceName, created)
	return nil
}

func (s *composeService) executePlanStartContainer(ctx context.Context, op *Operation) error {
	return s.startContainer(ctx, *op.ContainerOp.Existing)
}

func (s *composeService) executePlanStopContainer(ctx context.Context, op *Operation) error {
	return s.stopContainer(ctx, nil, *op.ContainerOp.Existing, op.ContainerOp.Timeout, nil)
}

func (s *composeService) executePlanRemoveContainer(ctx context.Context, op *Operation) error {
	service := op.ContainerOp.Service
	return s.stopAndRemoveContainer(ctx, *op.ContainerOp.Existing, &service, op.ContainerOp.Timeout, false)
}

func (s *composeService) executePlanRunPlugin(ctx context.Context, project *types.Project, op *Operation) error {
	return s.runPlugin(ctx, project, op.PluginOp.Service, op.PluginOp.Action)
}

// DisplayPlan performs a topological sort of operations and displays them
// grouped by resource type.
func DisplayPlan(plan *ReconciliationPlan, w io.Writer) error {
	ops := topologicalSort(plan)

	// Group operations by category
	var networkOps, volumeOps []*Operation
	serviceOps := make(map[string][]*Operation)

	for _, op := range ops {
		switch {
		case op.NetworkOp != nil:
			networkOps = append(networkOps, op)
		case op.VolumeOp != nil:
			volumeOps = append(volumeOps, op)
		case op.ContainerOp != nil:
			serviceOps[op.ServiceName] = append(serviceOps[op.ServiceName], op)
		case op.PluginOp != nil:
			serviceOps[op.ServiceName] = append(serviceOps[op.ServiceName], op)
		}
	}

	// Display networks
	if len(networkOps) > 0 {
		if _, err := fmt.Fprintln(w, "Networks:"); err != nil {
			return err
		}
		for _, op := range networkOps {
			name := op.Resource
			if _, err := fmt.Fprintf(w, "  [%-10s] %-20s reason: %s\n", opVerb(op.Type), name, op.Reason); err != nil {
				return err
			}
		}
	}

	// Display volumes
	if len(volumeOps) > 0 {
		if _, err := fmt.Fprintln(w, "Volumes:"); err != nil {
			return err
		}
		for _, op := range volumeOps {
			name := op.Resource
			if _, err := fmt.Fprintf(w, "  [%-10s] %-20s reason: %s\n", opVerb(op.Type), name, op.Reason); err != nil {
				return err
			}
		}
	}

	// Display services
	if len(serviceOps) > 0 {
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
				name := op.Resource
				if _, err := fmt.Fprintf(w, "    [%-10s] %-20s reason: %s\n", opVerb(op.Type), name, op.Reason); err != nil {
					return err
				}
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
	case OpRecreateNetwork, OpRecreateVolume, OpRecreateContainer:
		return "recreate"
	case OpRemoveNetwork, OpRemoveVolume, OpRemoveContainer:
		return "remove"
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
func topologicalSort(plan *ReconciliationPlan) []*Operation {
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

	return sorted
}
