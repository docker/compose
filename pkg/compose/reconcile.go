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
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	mmount "github.com/moby/moby/api/types/mount"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
)

// toReconcileOptions maps api.CreateOptions to ReconcileOptions.
func toReconcileOptions(options api.CreateOptions) ReconcileOptions {
	return ReconcileOptions{
		Services:             options.Services,
		Recreate:             options.Recreate,
		RecreateDependencies: options.RecreateDependencies,
		Inherit:              options.Inherit,
		Timeout:              options.Timeout,
		RemoveOrphans:        options.RemoveOrphans,
		SkipProviders:        options.SkipProviders,
	}
}

// ReconcileOptions controls how the reconciler compares desired and observed state.
type ReconcileOptions struct {
	Services             []string       // targeted services (empty = all)
	Recreate             string         // "diverged", "force", "never" for targeted services
	RecreateDependencies string         // same for non-targeted services
	Inherit              bool           // inherit anonymous volumes on recreate
	Timeout              *time.Duration // for stop operations
	RemoveOrphans        bool
	SkipProviders        bool
	// Start, when set, extends the plan with the start phase:
	// dependency-condition waits, pre_start hooks and container starts. When
	// nil the plan stops at container creation and starting is left to the
	// caller (the historical two-engine split).
	Start *startPhaseOptions
}

// startPhaseOptions configures the start phase of a plan.
type startPhaseOptions struct {
	// Services restricts which services get start-phase nodes (nil = all).
	// Used by watch, which only restarts the rebuilt services and their
	// dependents.
	Services []string
	// WaitTimeout bounds each dependency-condition wait (0 = no deadline).
	WaitTimeout time.Duration
	// Only plans the start phase alone — the `compose start` profile: no
	// network/volume reconciliation, no container creation, recreation or
	// removal; existing containers are started as they are.
	Only bool
}

// reconciler compares a types.Project (desired state) with an ObservedState
// (actual state) and produces a Plan — a DAG of atomic operations.
type reconciler struct {
	project  *types.Project
	observed *ObservedState
	options  ReconcileOptions
	// prompt interacts with the user to confirm destructive decisions taken
	// while building the plan. Today its only consumer is reconcileVolumes,
	// which asks for confirmation before scheduling the recreation of a volume
	// whose configuration has diverged (an operation that loses the volume's
	// data). Network recreation is not gated: it is not destructive.
	prompt Prompt
	plan   *Plan

	// networkNodes and volumeNodes track the last plan node for each
	// network/volume, so container creation nodes can depend on them.
	networkNodes map[string]*PlanNode // compose network key → create node
	volumeNodes  map[string]*PlanNode // compose volume key → create node
	// serviceNodes tracks the last plan node per service, so dependent
	// services can order their operations after dependencies.
	serviceNodes map[string]*PlanNode
	// stoppedByPlan records containers already stopped by an earlier stage
	// of the plan (typically planRecreateNetwork) so that downstream stages
	// can chain on the existing OpStopContainer instead of emitting a second
	// one against an already-stopped container.
	stoppedByPlan map[string]*PlanNode // container ID → existing Stop node

	// connectNodes records the OpConnectNetwork nodes emitted for a container by
	// planRecreateNetworks (reconnecting it to a freshly recreated network). If
	// reconcileContainers later recreates the same container, its RemoveContainer
	// must wait for these reconnects so they don't race the removal.
	connectNodes map[string][]*PlanNode // container ID → reconnect nodes

	// recreatedServices is the set of services with at least one container
	// scheduled for recreation in the current plan. Services iterate in
	// dependency order, so by the time a dependent is evaluated, all its
	// parents are final — read by parentNamespaceRecreated to cascade
	// recreates to dependents that would otherwise hold stale
	// "container:<old_id>" references.
	recreatedServices map[string]bool

	// observedContainersByService memoizes ObservedState.containersByService()
	// (an O(services * containers) build) for expectedConfigHash, which is
	// called once per service.
	observedContainersByService map[string]Containers

	// startNodes tracks, per service, the start-phase nodes emitted when
	// options.Start is set. Dependents chain their own start phase after
	// these (a service is "started" once all its replicas are).
	startNodes map[string][]*PlanNode

	// waitConditionNodes dedupes WaitCondition nodes: several dependents
	// waiting on the same dependency with the same condition and the same
	// required-ness share one polling node. required stays in the key because
	// Optional is a node property: a required and an optional edge to the
	// same dependency do not fail the same way.
	waitConditionNodes map[string]*PlanNode

	// resolvedNetworks/resolvedVolumes hold the single live resource selected per
	// compose key from the (possibly multi-valued) observed state — see
	// resolveObserved. All reconcile logic reads these, never observed.Networks/
	// observed.Volumes directly, so selection happens exactly once.
	resolvedNetworks map[string]ObservedNetwork
	resolvedVolumes  map[string]ObservedVolume
}

// reconcile is the main entry point: it builds a Plan from desired vs observed state.
// The prompt function is consulted while planning to confirm destructive
// decisions (see the reconciler.prompt field).
func reconcile(_ context.Context, project *types.Project, observed *ObservedState, options ReconcileOptions, prompt Prompt) (*Plan, error) {
	r := &reconciler{
		project:                     project,
		observed:                    observed,
		options:                     options,
		prompt:                      prompt,
		plan:                        &Plan{},
		networkNodes:                map[string]*PlanNode{},
		volumeNodes:                 map[string]*PlanNode{},
		serviceNodes:                map[string]*PlanNode{},
		stoppedByPlan:               map[string]*PlanNode{},
		connectNodes:                map[string][]*PlanNode{},
		recreatedServices:           map[string]bool{},
		observedContainersByService: observed.containersByService(),
		startNodes:                  map[string][]*PlanNode{},
		waitConditionNodes:          map[string]*PlanNode{},
	}

	if options.Start != nil && options.Start.Only {
		// `compose start` profile: only the start phase is planned, resources
		// and containers are taken as they are.
		if err := r.reconcileContainers(); err != nil {
			return nil, err
		}
		return r.plan, nil
	}

	r.resolveObserved()

	if err := r.reconcileNetworks(); err != nil {
		return nil, err
	}

	if err := r.reconcileVolumes(); err != nil {
		return nil, err
	}

	if err := r.reconcileContainers(); err != nil {
		return nil, err
	}

	if r.options.RemoveOrphans {
		r.reconcileOrphans()
	}

	return r.plan, nil
}

// resolveObserved selects, for every declared network and volume, the single
// live resource that matches it best (see selectNetwork/selectVolume) and stores
// it in resolvedNetworks/resolvedVolumes — the only observed views the rest of
// the reconciler reads. Extra live resources sharing a compose key (typically a
// leftover after a rename) are reported as orphans: they are left untouched —
// removing them could drop data or break unrelated workloads — but the user is
// warned so they can clean up, and selection stays deterministic across runs.
func (r *reconciler) resolveObserved() {
	r.resolvedNetworks = make(map[string]ObservedNetwork, len(r.project.Networks))
	for _, key := range sortedKeys(r.project.Networks) {
		selected, orphans, ok := r.observed.selectNetwork(key, r.project.Networks[key].Name)
		if !ok {
			continue
		}
		r.resolvedNetworks[key] = selected
		for _, o := range orphans {
			logrus.Warnf("network %q (id %s) carries the compose label %q but does not match the compose file (using %q); "+
				"it is left untouched — remove it manually if it is no longer needed", o.Name, o.ID, key, selected.Name)
		}
	}
	r.resolvedVolumes = make(map[string]ObservedVolume, len(r.project.Volumes))
	for _, key := range sortedKeys(r.project.Volumes) {
		selected, orphans, ok := r.observed.selectVolume(key, r.project.Volumes[key].Name)
		if !ok {
			continue
		}
		r.resolvedVolumes[key] = selected
		for _, o := range orphans {
			logrus.Warnf("volume %q carries the compose label %q but does not match the compose file (using %q); "+
				"it is left untouched — remove it manually if it is no longer needed", o.Name, key, selected.Name)
		}
	}
}

// reconcileNetworks plans the network lifecycle: creation of missing networks
// and, for networks whose configuration has diverged from the live resource,
// recreation. Unlike volumes, recreating a network is not destructive, so no
// user confirmation is required.
//
// Divergence is detected by comparing NetworkHash(desired) with the config-hash
// persisted on the live network (observed.ConfigHash). A network with no
// recorded hash (e.g. created by an older Compose or manually) is left
// untouched, matching the previous ensureNetwork behavior.
//
// A rename (observed.Name != desired.Name) also diverges the hash — NetworkHash
// includes the name — and is handled by the same recreation path: the old
// network is removed and the new one created, migrating attached containers onto
// it. Networks carry no data, so removing the previous network (rather than
// leaving it dangling) is safe and keeps subsequent runs deterministic.
func (r *reconciler) reconcileNetworks() error {
	var diverged []string
	for _, key := range sortedKeys(r.project.Networks) {
		desired := r.project.Networks[key]
		if desired.External {
			continue
		}
		observed, exists := r.resolvedNetworks[key]
		if !exists {
			r.planCreateNetwork(key, &desired, "not found")
			continue
		}
		expected, err := NetworkHash(&desired)
		if err != nil {
			return err
		}
		if observed.ConfigHash == "" || observed.ConfigHash == expected {
			continue
		}
		diverged = append(diverged, key)
	}
	r.planRecreateNetworks(diverged)
	return nil
}

// planCreateNetwork adds a single CreateNetwork node and records it for dependency tracking.
func (r *reconciler) planCreateNetwork(key string, nw *types.NetworkConfig, cause string) {
	r.networkNodes[key] = r.plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: fmt.Sprintf("network:%s", key),
		Cause:      cause,
		Name:       nw.Name,
		Network:    nw,
	}, "")
}

// planRecreateNetworks adds, for each diverged network, the sequence:
//
//	stop containers → disconnect containers → remove network → create network → connect containers
//
// Attached containers must be disconnected before the network can be removed
// (Docker refuses to remove a network with active endpoints) and are reconnected
// to the fresh network afterwards — they keep their identity and are not
// recreated, matching the previous ensureNetwork/removeDivergedNetwork behavior.
//
// Stops are deduplicated through stoppedByPlan so a container attached to several
// diverged networks (or later recreated by reconcileContainers) is stopped once.
// Each reconnect is recorded in connectNodes so that, should reconcileContainers
// recreate the container for an unrelated reason, its removal is ordered after
// the reconnect instead of racing it.
func (r *reconciler) planRecreateNetworks(keys []string) {
	for _, key := range keys {
		observed := r.resolvedNetworks[key]
		desired := r.project.Networks[key]
		containers := r.containersForServices(r.servicesUsingNetwork(key))

		// Stop then disconnect every attached container.
		var disconnectNodes []*PlanNode
		for i := range containers {
			oc := &containers[i]
			resID := fmt.Sprintf("service:%s:%d", oc.Summary.Labels[api.ServiceLabel], oc.Number)
			stopNode, alreadyStopped := r.stoppedByPlan[oc.ID]
			if !alreadyStopped {
				stopNode = r.plan.addNode(Operation{
					Type:       OpStopContainer,
					ResourceID: resID,
					Cause:      fmt.Sprintf("network %s config changed", key),
					Container:  &oc.Summary,
					Timeout:    r.options.Timeout,
				}, "")
				r.stoppedByPlan[oc.ID] = stopNode
			}
			disconnectNodes = append(disconnectNodes, r.plan.addNode(Operation{
				Type:       OpDisconnectNetwork,
				ResourceID: resID,
				Cause:      fmt.Sprintf("network %s recreate", key),
				Container:  &oc.Summary,
				Name:       observed.Name,
			}, "", stopNode))
		}

		// A rename (the live network has a different name than desired) does not
		// require removing the old network: the new one has a distinct name, so
		// it is created independently and the old removal becomes best-effort
		// cleanup — skipped with a warning if the network is still in use by
		// non-Compose containers, instead of blocking the whole operation. A
		// same-name divergence, on the other hand, must remove the old network
		// before the new one can be created.
		rename := observed.Name != desired.Name
		removeCause := "config hash diverged"
		createCause := "recreate after config change"
		if rename {
			removeCause = "renamed (best-effort cleanup)"
			createCause = "renamed"
		}

		removeNode := r.plan.addNode(Operation{
			Type:       OpRemoveNetwork,
			ResourceID: fmt.Sprintf("network:%s", key),
			Cause:      removeCause,
			Name:       observed.Name,
			BestEffort: rename,
		}, "", disconnectNodes...)

		var createDeps []*PlanNode
		if !rename {
			createDeps = []*PlanNode{removeNode}
		}
		createNode := r.plan.addNode(Operation{
			Type:       OpCreateNetwork,
			ResourceID: fmt.Sprintf("network:%s", key),
			Cause:      createCause,
			Name:       desired.Name,
			Network:    &desired,
		}, "", createDeps...)
		r.networkNodes[key] = createNode

		// Reconnect every attached container to the fresh network. On a rename the
		// reconnect also waits for the container to be disconnected from the old
		// network first (on a same-name recreate that ordering already holds
		// transitively through remove → create).
		for i := range containers {
			oc := &containers[i]
			resID := fmt.Sprintf("service:%s:%d", oc.Summary.Labels[api.ServiceLabel], oc.Number)
			deps := []*PlanNode{createNode}
			if rename {
				deps = append(deps, disconnectNodes[i])
			}
			connectNode := r.plan.addNode(Operation{
				Type:       OpConnectNetwork,
				ResourceID: resID,
				Cause:      fmt.Sprintf("network %s recreate", key),
				Container:  &oc.Summary,
				Name:       desired.Name,
			}, "", deps...)
			r.connectNodes[oc.ID] = append(r.connectNodes[oc.ID], connectNode)
		}
	}
}

// reconcileVolumes plans the volume lifecycle: creation of missing volumes and,
// for volumes whose configuration has diverged from the live resource,
// recreation — gated on user confirmation because it destroys the volume's data.
//
// Divergence is detected by comparing VolumeHash(desired) with the config-hash
// persisted on the live volume (observed.ConfigHash). A volume with no recorded
// hash (e.g. created by an older Compose) is left untouched, matching the
// previous ensureVolume behavior.
func (r *reconciler) reconcileVolumes() error {
	var diverged []string
	for _, key := range sortedKeys(r.project.Volumes) {
		desired := r.project.Volumes[key]
		if desired.External {
			continue
		}
		observed, exists := r.resolvedVolumes[key]
		if !exists {
			r.planCreateVolume(key, &desired, "not found")
			continue
		}
		expected, err := VolumeHash(desired)
		if err != nil {
			return err
		}
		if observed.ConfigHash == "" || observed.ConfigHash == expected {
			continue
		}
		if observed.Name != desired.Name {
			// The volume was renamed: the live volume matched by label carries a
			// different name, i.e. a distinct Docker resource. Match the
			// historical additive behavior — create the new volume and leave the
			// old one (and its data) untouched — instead of prompting to delete
			// data under a name that does not exist yet.
			r.planCreateVolume(key, &desired, "renamed")
			// Rewrite the observed name to the desired one so reconcileContainers
			// detects the mount mismatch and migrates existing containers onto
			// the new volume within the same up (as the pre-reconcile ensureVolume
			// path did), and so later runs match deterministically on the new
			// name rather than split-braining between the two.
			observed.Name = desired.Name
			r.resolvedVolumes[key] = observed
			continue
		}
		confirmed, err := r.prompt(
			fmt.Sprintf("Volume %q exists but doesn't match configuration in compose file. Recreate (data will be lost)?", desired.Name),
			false)
		if err != nil {
			return err
		}
		if confirmed {
			diverged = append(diverged, key)
		}
	}
	r.planRecreateVolumes(diverged)
	return nil
}

// planCreateVolume adds a single CreateVolume node and records it for dependency tracking.
func (r *reconciler) planCreateVolume(key string, vol *types.VolumeConfig, cause string) {
	r.volumeNodes[key] = r.plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: fmt.Sprintf("volume:%s", key),
		Cause:      cause,
		Name:       vol.Name,
		Volume:     vol,
	}, "")
}

// planRecreateVolumes schedules the recreation of the given (confirmed) diverged
// volumes and hands the re-creation of the impacted service containers to
// reconcileContainers. The resulting plan, for each affected container/volume, is:
//
//	stop containers → remove containers → remove volume → create volume → create containers
//
// Containers must be *removed* (not merely stopped) before a volume can be
// removed: Docker refuses to remove a volume still referenced by any container,
// even a stopped one. They are then recreated once the fresh volume exists.
//
// Rather than re-implementing container creation here, the affected services are
// cleared from the observed snapshot: reconcileContainers (which runs next) then
// sees them as absent and schedules fresh containers that depend on the
// CreateVolume node via infrastructureDeps. Marking those services as recreated
// propagates the cascade to namespace/volume-sharing dependents.
//
// Container stops/removes are planned once per container even when a container
// mounts several diverged volumes, and every RemoveVolume waits for all affected
// container removals, so the ordering holds regardless of which service mounts
// which volume.
func (r *reconciler) planRecreateVolumes(keys []string) {
	if len(keys) == 0 {
		return
	}

	// Collect the services (and their containers) mounting any diverged volume.
	serviceSet := map[string]bool{}
	for _, key := range keys {
		for _, svc := range r.servicesUsingVolume(key) {
			serviceSet[svc] = true
		}
	}
	services := sortedKeys(serviceSet)
	containers := r.containersForServices(services)

	// Stop then remove every affected container.
	var removeNodes []*PlanNode
	for i := range containers {
		oc := &containers[i]
		resID := fmt.Sprintf("service:%s:%d", oc.Summary.Labels[api.ServiceLabel], oc.Number)
		stopNode, alreadyStopped := r.stoppedByPlan[oc.ID]
		if !alreadyStopped {
			stopNode = r.plan.addNode(Operation{
				Type:       OpStopContainer,
				ResourceID: resID,
				Cause:      "mounted volume config changed",
				Container:  &oc.Summary,
				Timeout:    r.options.Timeout,
			}, "")
			r.stoppedByPlan[oc.ID] = stopNode
		}
		removeNode := r.plan.addNode(Operation{
			Type:       OpRemoveContainer,
			ResourceID: resID,
			Cause:      "mounted volume config changed",
			Container:  &oc.Summary,
		}, "", stopNode)
		removeNodes = append(removeNodes, removeNode)
	}

	// Remove then recreate each diverged volume once all affected containers are
	// gone. Record the CreateVolume node so the fresh containers scheduled by
	// reconcileContainers depend on it (via infrastructureDeps).
	for _, key := range keys {
		desired := r.project.Volumes[key]
		removeVolNode := r.plan.addNode(Operation{
			Type:       OpRemoveVolume,
			ResourceID: fmt.Sprintf("volume:%s", key),
			Cause:      "config hash diverged",
			Name:       r.resolvedVolumes[key].Name,
		}, "", removeNodes...)
		createVolNode := r.plan.addNode(Operation{
			Type:       OpCreateVolume,
			ResourceID: fmt.Sprintf("volume:%s", key),
			Cause:      "recreate after config change",
			Name:       desired.Name,
			Volume:     &desired,
		}, "", removeVolNode)
		r.volumeNodes[key] = createVolNode
	}

	// Hand container re-creation to reconcileContainers: cleared services are
	// seen as absent and scheduled fresh (gated on their CreateVolume node via
	// infrastructureDeps), and marking them recreated cascades to
	// namespace/volume-sharing dependents.
	//
	// Only observed.Containers is cleared, not the observedContainersByService
	// snapshot memoized at reconciler init: that snapshot backs config-hash
	// resolution (serviceHashWithResolvedRefs), which must mirror the state the
	// executor hashed against at create time, whereas clearing here is purely a
	// scheduling concern carried by the plan's dependency edges. The two
	// intentionally diverge; do not "fix" one to match the other.
	for _, svc := range services {
		r.recreatedServices[svc] = true
		r.observed.Containers[svc] = nil
	}
}

// servicesUsingNetwork returns the names of services that reference the given
// compose network key, sorted for deterministic plan output.
func (r *reconciler) servicesUsingNetwork(networkKey string) []string {
	var names []string
	for _, key := range sortedKeys(r.project.Services) {
		svc := r.project.Services[key]
		if _, ok := svc.Networks[networkKey]; ok {
			names = append(names, svc.Name)
		}
	}
	return names
}

// servicesUsingVolume returns the names of services whose containers reference
// the given compose volume — either by mounting it directly (service.Volumes) or
// by inheriting the mount transitively through volumes_from. Every such service's
// containers must be removed before the volume can be removed: Docker refuses to
// remove a volume still referenced by any container, and volumes_from
// materializes the source's mounts on the target container. Sorted for
// deterministic plan output.
//
// Only volumes_from propagates a *mount* (and therefore a volume reference);
// network_mode/ipc/pid: service:x share namespaces, not mounts, so they do not
// keep a volume in use and are intentionally excluded here.
func (r *reconciler) servicesUsingVolume(volumeKey string) []string {
	inSet := map[string]bool{}
	// Seed with services that mount the volume directly.
	for _, key := range sortedKeys(r.project.Services) {
		svc := r.project.Services[key]
		for _, v := range svc.Volumes {
			if v.Source == volumeKey {
				inSet[svc.Name] = true
				break
			}
		}
	}
	// Grow the set by transitive volumes_from closure until it stabilizes: a
	// service inherits the mount when it draws volumes from a service already in
	// the set (references to external containers carry no compose dependency).
	for {
		added := false
		for _, key := range sortedKeys(r.project.Services) {
			svc := r.project.Services[key]
			if inSet[svc.Name] {
				continue
			}
			for _, vf := range svc.VolumesFrom {
				if strings.HasPrefix(vf, types.ContainerPrefix) {
					continue
				}
				name, _, _ := strings.Cut(vf, ":")
				if inSet[name] {
					inSet[svc.Name] = true
					added = true
					break
				}
			}
		}
		if !added {
			break
		}
	}
	return sortedKeys(inSet)
}

// containersForServices returns all observed containers belonging to the given
// service names.
func (r *reconciler) containersForServices(services []string) []ObservedContainer {
	var result []ObservedContainer
	for _, svc := range services {
		result = append(result, r.observed.Containers[svc]...)
	}
	return result
}

// reconcileContainers processes each service in dependency order, comparing
// the desired scale and configuration with observed containers.
func (r *reconciler) reconcileContainers() error {
	// Build dependency graph and process in order
	graph, err := NewGraph(r.project, ServiceStopped)
	if err != nil {
		return err
	}

	// Visit in dependency order (leaves first = services with no deps)
	return r.visitInDependencyOrder(graph)
}

// visitInDependencyOrder processes services from leaves to roots so that
// dependencies are reconciled before the services that depend on them.
func (r *reconciler) visitInDependencyOrder(g *Graph) error {
	visited := map[string]bool{}
	// Sort vertex keys for deterministic plan output in tests
	keys := sortedKeys(g.Vertices)
	for {
		// Find a vertex whose all children are visited
		var next *Vertex
		for _, k := range keys {
			v := g.Vertices[k]
			if visited[v.Key] {
				continue
			}
			allChildrenVisited := true
			for _, child := range v.Children {
				if !visited[child.Key] {
					allChildrenVisited = false
					break
				}
			}
			if allChildrenVisited {
				next = v
				break
			}
		}
		if next == nil {
			break // all visited
		}
		visited[next.Key] = true

		service, err := r.project.GetService(next.Service)
		if err != nil {
			return err
		}
		if err := r.reconcileService(service); err != nil {
			return err
		}
	}
	return nil
}

// reconcileService handles a single service: scale down, recreate diverged,
// start stopped, scale up.
func (r *reconciler) reconcileService(service types.ServiceConfig) error {
	if r.options.Start != nil && r.options.Start.Only {
		return r.reconcileServiceStartOnly(service)
	}
	if service.Provider != nil && r.options.SkipProviders {
		return nil
	}
	if service.Provider != nil {
		svc := service
		deps := r.infrastructureDeps(service)
		node := r.plan.addNode(Operation{
			Type:       OpRunProvider,
			ResourceID: fmt.Sprintf("provider:%s", service.Name),
			Cause:      "provider service",
			Service:    &svc,
		}, "", deps...)
		r.serviceNodes[service.Name] = node
		return nil
	}

	expected, err := getScale(service)
	if err != nil {
		return err
	}

	containers := r.observed.Containers[service.Name]
	actual := len(containers)

	strategy := r.options.RecreateDependencies
	if slices.Contains(r.options.Services, service.Name) || len(r.options.Services) == 0 {
		strategy = r.options.Recreate
	}

	// Precompute once per service: mustRecreate is called twice per container
	// (sortContainers + main loop) and the hash/cascade inputs depend on the
	// service, not the container.
	expectedHash, err := serviceHashWithResolvedRefs(service, r.observedContainersByService)
	if err != nil {
		return err
	}
	parentRecreated := r.parentNamespaceRecreated(service)

	// Sort containers: obsolete first, then by number descending, then reverse
	// to get the same ordering as the existing convergence code.
	r.sortContainers(containers, service, expectedHash, parentRecreated, strategy)

	// Collect dependency nodes that container creation should depend on
	infraDeps := r.infrastructureDeps(service)

	lastNode, candidates := r.planExistingContainers(service, containers, expected, strategy, expectedHash, parentRecreated, infraDeps)

	// Scale up: create new containers
	nextNum := nextContainerNumber(r.observedSummaries(service.Name))
	for i := 0; i < expected-actual; i++ {
		number := nextNum + i
		name := getContainerName(r.project.Name, service, number)
		svc := service // copy for pointer stability
		lastNode = r.plan.addNode(Operation{
			Type:       OpCreateContainer,
			ResourceID: fmt.Sprintf("service:%s:%d", service.Name, number),
			Cause:      "no existing container",
			Service:    &svc,
			Number:     number,
			Name:       name,
		}, "", infraDeps...)
		candidates = append(candidates, startCandidate{
			number:       number,
			after:        []*PlanNode{lastNode},
			createNodeID: lastNode.ID,
		})
	}

	if r.options.Start != nil {
		node, err := r.planServiceStart(service, candidates)
		if err != nil {
			return err
		}
		if node != nil {
			lastNode = node
		}
	}

	if lastNode != nil {
		r.serviceNodes[service.Name] = lastNode
	}
	return nil
}

// reconcileServiceStartOnly is the `compose start` profile of reconcileService:
// existing containers are taken as they are — no creation, recreation or
// removal — and only the start phase (waits, pre_start, starts) is planned.
func (r *reconciler) reconcileServiceStartOnly(service types.ServiceConfig) error {
	if service.Provider != nil {
		return nil
	}
	var candidates []startCandidate
	for i, oc := range r.observed.Containers[service.Name] {
		if oc.State == container.StateRunning {
			candidates = append(candidates, startCandidate{number: oc.Number, running: true})
			continue
		}
		candidates = append(candidates, startCandidate{
			number:    oc.Number,
			container: &r.observed.Containers[service.Name][i].Summary,
		})
	}
	node, err := r.planServiceStart(service, candidates)
	if err != nil {
		return err
	}
	if node != nil {
		r.serviceNodes[service.Name] = node
	}
	return nil
}

// planExistingContainers walks the service's observed containers (pre-sorted
// by sortContainers) and plans scale-downs, recreations and paused/dead
// restarts. It returns the last node emitted and the start candidates the
// start phase will act on when options.PlanStart is set.
func (r *reconciler) planExistingContainers(
	service types.ServiceConfig, containers []ObservedContainer, expected int,
	strategy, expectedHash string, parentRecreated bool, infraDeps []*PlanNode,
) (*PlanNode, []startCandidate) {
	var lastNode *PlanNode
	var candidates []startCandidate
	for i, oc := range containers {
		if i >= expected {
			// Scale down: stop + remove excess containers. Track the remove
			// node so dependent services wait for the scale-down to finish
			// even when no other operation runs on this service.
			stopNode := r.plan.addNode(Operation{
				Type:       OpStopContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", service.Name, oc.Number),
				Cause:      "scale down",
				Container:  &containers[i].Summary,
				Timeout:    r.options.Timeout,
			}, "")
			lastNode = r.plan.addNode(Operation{
				Type:       OpRemoveContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", service.Name, oc.Number),
				Cause:      "scale down",
				Container:  &containers[i].Summary,
			}, "", stopNode)
			continue
		}

		if r.mustRecreate(service, expectedHash, parentRecreated, oc, strategy) {
			var createNode *PlanNode
			lastNode, createNode = r.planRecreateContainer(service, &containers[i], infraDeps)
			r.recreatedServices[service.Name] = true
			candidates = append(candidates, startCandidate{
				number:       oc.Number,
				after:        []*PlanNode{lastNode},
				createNodeID: createNode.ID,
			})
			continue
		}

		// Container is up-to-date
		switch oc.State {
		case container.StateRunning, container.StateRestarting:
			if stopNode, stopped := r.stoppedByPlan[oc.ID]; stopped {
				// The plan stops this container (e.g. to recreate a diverged
				// network): it must be started again once reconnected.
				after := append([]*PlanNode{stopNode}, r.connectNodes[oc.ID]...)
				candidates = append(candidates, startCandidate{
					number:    oc.Number,
					after:     after,
					container: &containers[i].Summary,
				})
			} else {
				candidates = append(candidates, startCandidate{number: oc.Number, running: true})
			}
		case container.StateCreated, container.StateExited:
			// Nothing to plan on the create side: starting created/exited
			// containers is the start phase's job — planned by
			// planServiceStart when PlanStart is set, left to the caller's
			// start engine otherwise.
			candidates = append(candidates, startCandidate{
				number:    oc.Number,
				after:     r.connectNodes[oc.ID],
				container: &containers[i].Summary,
			})
		default:
			// Any other state (paused, dead, ...): attempt to (re)start
			lastNode = r.plan.addNode(Operation{
				Type:       OpStartContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", service.Name, oc.Number),
				Cause:      "not running",
				Container:  &containers[i].Summary,
			}, "", infraDeps...)
		}
	}
	return lastNode, candidates
}

// startCandidate describes one replica the start phase may have to start:
// either an existing container (container set) or one the plan creates
// (createNodeID set, resolved by the executor once the create ran).
type startCandidate struct {
	number       int
	after        []*PlanNode        // nodes the start must wait for (create, reconnects, ...)
	container    *container.Summary // existing container, nil for plan-created ones
	createNodeID int                // create node whose result carries the container ID
	running      bool               // already running and left untouched: nothing to start
}

// planServiceStart extends the plan with the service's start phase:
// dependency-condition waits, pre_start hooks and one start per non-running
// replica. Returns the last node emitted, or nil if nothing needed starting.
func (r *reconciler) planServiceStart(service types.ServiceConfig, candidates []startCandidate) (*PlanNode, error) {
	if r.options.Start.Services != nil && !slices.Contains(r.options.Start.Services, service.Name) {
		return nil, nil
	}
	svc := service // copy for pointer stability

	var toStart []startCandidate
	anyRunning := false
	for _, c := range candidates {
		if c.running {
			anyRunning = true
			continue
		}
		toStart = append(toStart, c)
	}
	if len(toStart) == 0 {
		return nil, nil
	}

	startDeps, err := r.planDependencyWaits(service)
	if err != nil {
		return nil, err
	}

	// pre_start hooks run once per service, only when no replica is already
	// running, on the lowest-numbered replica — decided here at plan time
	// from the observed state, accepting the same observe-to-execute drift
	// window as the rest of the plan.
	var preStart *PlanNode
	if len(service.PreStart) > 0 && !anyRunning {
		lowest := toStart[0]
		for _, c := range toStart[1:] {
			if c.number < lowest.number {
				lowest = c
			}
		}
		deps := append(slices.Clone(startDeps), lowest.after...)
		preStart = r.plan.addNode(Operation{
			Type:         OpRunPreStart,
			ResourceID:   fmt.Sprintf("prestart:%s", service.Name),
			Cause:        "pre_start hooks",
			Service:      &svc,
			Number:       lowest.number,
			Container:    lowest.container,
			CreateNodeID: lowest.createNodeID,
		}, "", deps...)
	}

	var last *PlanNode
	for _, c := range toStart {
		deps := append(slices.Clone(startDeps), c.after...)
		if preStart != nil {
			deps = append(deps, preStart)
		}
		last = r.plan.addNode(Operation{
			Type:         OpStartContainer,
			ResourceID:   fmt.Sprintf("service:%s:%d", service.Name, c.number),
			Cause:        "service start",
			Service:      &svc,
			Name:         getContainerName(r.project.Name, service, c.number),
			Number:       c.number,
			Container:    c.container,
			CreateNodeID: c.createNodeID,
		}, "", deps...)
		r.startNodes[service.Name] = append(r.startNodes[service.Name], last)
	}
	return last, nil
}

// planDependencyWaits returns the nodes a service's start must depend on for
// its depends_on edges: the dependencies' own start nodes (which alone express
// the service_started condition) plus one WaitCondition node per conditional
// edge. Dependents waiting on the same dependency with the same condition and
// required-ness share one polling node.
func (r *reconciler) planDependencyWaits(service types.ServiceConfig) ([]*PlanNode, error) {
	var startDeps []*PlanNode
	for _, dep := range sortedKeys(service.DependsOn) {
		config := service.DependsOn[dep]
		depNodes := r.startNodes[dep]
		if len(depNodes) == 0 {
			if node := r.serviceNodes[dep]; node != nil {
				depNodes = []*PlanNode{node}
			}
		}
		startDeps = append(startDeps, depNodes...)

		shouldWait, err := shouldWaitForDependency(dep, config, r.project)
		if err != nil {
			return nil, err
		}
		if !shouldWait {
			continue
		}
		key := fmt.Sprintf("%s:%s:%t", dep, config.Condition, config.Required)
		if node, ok := r.waitConditionNodes[key]; ok {
			startDeps = append(startDeps, node)
			continue
		}
		depSvc := r.project.Services[dep]
		node := r.plan.addNode(Operation{
			Type:       OpWaitCondition,
			ResourceID: fmt.Sprintf("wait:%s:%s", dep, config.Condition),
			Cause:      fmt.Sprintf("dependency %s must be %s", dep, config.Condition),
			Service:    &depSvc,
			Name:       dep,
			Condition:  config.Condition,
			Timeout:    waitTimeout(r.options.Start.WaitTimeout),
			Optional:   !config.Required,
		}, "", depNodes...)
		r.waitConditionNodes[key] = node
		startDeps = append(startDeps, node)
	}
	return startDeps, nil
}

func waitTimeout(d time.Duration) *time.Duration {
	if d <= 0 {
		return nil
	}
	return &d
}

// mustRecreate decides whether oc must be recreated to match expected. The
// expectedHash and parentRecreated inputs are precomputed once per service by
// reconcileService — see expectedConfigHash and parentNamespaceRecreated for
// the rationale (issue #13878).
func (r *reconciler) mustRecreate(expected types.ServiceConfig, expectedHash string, parentRecreated bool, oc ObservedContainer, policy string) bool {
	switch policy {
	case api.RecreateNever:
		return false
	case api.RecreateForce:
		return true
	}
	if parentRecreated {
		return true
	}
	if oc.ConfigHash != expectedHash {
		return true
	}
	if oc.ImageDigest != expected.CustomLabels[api.ImageDigestLabel] {
		return true
	}
	if oc.ImageVolumeDigest != expected.CustomLabels[api.ImageVolumeDigestLabel] {
		return true
	}
	if oc.State == container.StateRunning && r.hasNetworkMismatch(expected, oc) {
		return true
	}
	return r.hasVolumeMismatch(expected, oc)
}

// parentNamespaceRecreated reports whether any namespace- or volume-sharing
// parent of svc has at least one container scheduled for recreation. The
// parent set is derived from svc itself (network_mode/ipc/pid and volumes_from)
// rather than depends_on, so the cascade fires only when a stale
// "container:<id>" reference would otherwise be left behind.
func (r *reconciler) parentNamespaceRecreated(svc types.ServiceConfig) bool {
	for _, mode := range []string{svc.NetworkMode, svc.Ipc, svc.Pid} {
		if name := getDependentServiceFromMode(mode); name != "" && r.recreatedServices[name] {
			return true
		}
	}
	for _, vol := range svc.VolumesFrom {
		if strings.HasPrefix(vol, types.ContainerPrefix) {
			continue
		}
		name, _, _ := strings.Cut(vol, ":")
		if r.recreatedServices[name] {
			return true
		}
	}
	return false
}

// serviceHashWithResolvedRefs mirrors what the executor persists at create
// time: service references (network_mode/ipc/pid: service:X, volumes_from) are
// resolved against the observed containers snapshot before hashing. On
// resolution failure (e.g. referenced parent absent) the raw form is hashed —
// it cannot match the persisted hash either way, so recreation is forced.
//
// Only fields mutated by resolveServiceReferences need defensive copying.
// svc.Networks (a map) is left shared because resolveServiceReferences does
// not touch it; revisit if that changes.
func serviceHashWithResolvedRefs(svc types.ServiceConfig, containers map[string]Containers) (string, error) {
	resolved := svc
	resolved.VolumesFrom = slices.Clone(svc.VolumesFrom)
	_ = resolveServiceReferences(&resolved, containers)
	return ServiceHash(resolved)
}

// hasNetworkMismatch checks if the container is not connected to all expected networks.
func (r *reconciler) hasNetworkMismatch(expected types.ServiceConfig, oc ObservedContainer) bool {
	for _, net := range sortedKeys(expected.Networks) {
		expectedID := ""
		if obs, ok := r.resolvedNetworks[net]; ok {
			expectedID = obs.ID
		}
		if expectedID == "" || expectedID == "swarm" {
			continue
		}
		found := false
		for _, netID := range oc.ConnectedNetworks {
			if netID == expectedID {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// hasVolumeMismatch checks if the container is missing any expected volume mounts.
func (r *reconciler) hasVolumeMismatch(expected types.ServiceConfig, oc ObservedContainer) bool {
	for _, vol := range expected.Volumes {
		if vol.Type != string(mmount.TypeVolume) || vol.Source == "" {
			continue
		}
		expectedName := ""
		if obs, ok := r.resolvedVolumes[vol.Source]; ok {
			expectedName = obs.Name
		}
		if expectedName == "" {
			continue
		}
		found := false
		for _, m := range oc.Summary.Mounts {
			if m.Type == mmount.TypeVolume && m.Name == expectedName {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// planRecreateContainer decomposes container recreation into 4 atomic operations:
// CreateContainer(tmpName) → StopContainer → RemoveContainer → RenameContainer
// planRecreateContainer returns the final node of the recreate sequence and
// the create node (whose result carries the new container's ID, needed by the
// start phase).
func (r *reconciler) planRecreateContainer(service types.ServiceConfig, oc *ObservedContainer, infraDeps []*PlanNode) (*PlanNode, *PlanNode) {
	resID := fmt.Sprintf("service:%s:%d", service.Name, oc.Number)
	group := fmt.Sprintf("recreate:%s:%d", service.Name, oc.Number)
	tmpName := fmt.Sprintf("%s_%s", oc.ID[:min(12, len(oc.ID))], getContainerName(r.project.Name, service, oc.Number))
	svc := service // copy for pointer stability

	// Stop dependents first
	depStopNodes := r.planStopDependents(service)

	// All deps: infrastructure + dependent stops
	allDeps := append(slices.Clone(infraDeps), depStopNodes...)

	var inherited *container.Summary
	if r.options.Inherit {
		inherited = &oc.Summary
	}

	// 1. Create new container with temporary name
	createNode := r.plan.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: resID,
		Cause:      "config changed (tmpName)",
		Service:    &svc,
		Inherited:  inherited,
		Number:     oc.Number,
		Name:       tmpName,
	}, group, allDeps...)

	// 2. Stop old container. If an earlier stage of the plan (e.g.
	// planRecreateNetwork) already scheduled a Stop for this container,
	// reuse it instead of emitting a second one against an already-stopped
	// container. The reused node carries no group, which is fine: the
	// recreate's group tracker still drives Working/Done from the create.
	stopNode, alreadyStopped := r.stoppedByPlan[oc.ID]
	if !alreadyStopped {
		stopNode = r.plan.addNode(Operation{
			Type:       OpStopContainer,
			ResourceID: resID,
			Cause:      fmt.Sprintf("replaced by #%d", createNode.ID),
			Container:  &oc.Summary,
			Timeout:    r.options.Timeout,
		}, group, createNode)
		r.stoppedByPlan[oc.ID] = stopNode
	}

	// 3. Remove old container. The invariant is "new container exists before
	// old one is removed":
	//   - !alreadyStopped: stopNode is the one we just created with
	//     DependsOn=[createNode], so the chain remove → stop → create
	//     guarantees the invariant transitively.
	//   - alreadyStopped: stopNode comes from planRecreateNetwork and was
	//     emitted before createNode. The transitive guarantee no longer
	//     holds, so we add createNode to removeDeps explicitly. If anyone
	//     ever drops the stop → create edge from the !alreadyStopped case,
	//     they must add createNode here unconditionally.
	removeDeps := []*PlanNode{stopNode}
	if alreadyStopped {
		removeDeps = append(removeDeps, createNode)
	}
	// If planRecreateNetworks scheduled reconnects for this container (network
	// recreation), let them complete before the old container is removed so the
	// reconnect does not race the removal.
	removeDeps = append(removeDeps, r.connectNodes[oc.ID]...)
	removeNode := r.plan.addNode(Operation{
		Type:       OpRemoveContainer,
		ResourceID: resID,
		Cause:      fmt.Sprintf("replaced by #%d", createNode.ID),
		Container:  &oc.Summary,
	}, group, removeDeps...)

	// 4. Rename to final name. Link to the create node so the executor can
	// fetch the resulting container ID directly.
	finalName := getContainerName(r.project.Name, service, oc.Number)
	renameNode := r.plan.addNode(Operation{
		Type:         OpRenameContainer,
		ResourceID:   resID,
		Cause:        "finalize recreate",
		Name:         finalName,
		CreateNodeID: createNode.ID,
	}, group, removeNode)

	return renameNode, createNode
}

// planStopDependents plans stop operations for containers of services that
// depend on the given service with restart: true. Each emitted Stop is
// recorded in stoppedByPlan so a later planRecreateContainer for the same
// dependent reuses it instead of emitting a duplicate Stop.
func (r *reconciler) planStopDependents(service types.ServiceConfig) []*PlanNode {
	dependents := r.project.GetDependentsForService(service, func(dep types.ServiceDependency) bool {
		return dep.Restart
	})
	var nodes []*PlanNode
	for _, depName := range dependents {
		for i, oc := range r.observed.Containers[depName] {
			if _, already := r.stoppedByPlan[oc.ID]; already {
				continue
			}
			node := r.plan.addNode(Operation{
				Type:       OpStopContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", depName, oc.Number),
				Cause:      fmt.Sprintf("dependency %s being recreated", service.Name),
				Container:  &r.observed.Containers[depName][i].Summary,
				Timeout:    r.options.Timeout,
			}, "")
			r.stoppedByPlan[oc.ID] = node
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// infrastructureDeps returns the plan nodes that a container creation for this
// service should depend on: network creates and volume creates that the service
// references, plus the last node of dependency services.
func (r *reconciler) infrastructureDeps(service types.ServiceConfig) []*PlanNode {
	var deps []*PlanNode
	// Sort map keys for deterministic plan output in tests
	for _, net := range sortedKeys(service.Networks) {
		if node, ok := r.networkNodes[net]; ok {
			deps = append(deps, node)
		}
	}
	for _, vol := range service.Volumes {
		if vol.Type == string(mmount.TypeVolume) && vol.Source != "" {
			if node, ok := r.volumeNodes[vol.Source]; ok {
				deps = append(deps, node)
			}
		}
	}
	for _, depName := range sortedKeys(service.DependsOn) {
		if node, ok := r.serviceNodes[depName]; ok && node != nil {
			deps = append(deps, node)
		}
	}
	return deps
}

// sortContainers sorts containers the same way as convergence.go:138-160:
// obsolete first, then by container number descending, then reversed.
//
// mustRecreate is evaluated once per container before sorting to avoid
// quadratic re-evaluation in the comparator.
func (r *reconciler) sortContainers(containers []ObservedContainer, service types.ServiceConfig, expectedHash string, parentRecreated bool, policy string) {
	obsolete := make(map[string]bool, len(containers))
	for _, oc := range containers {
		obsolete[oc.ID] = r.mustRecreate(service, expectedHash, parentRecreated, oc, policy)
	}
	sort.Slice(containers, func(i, j int) bool {
		obsi, obsj := obsolete[containers[i].ID], obsolete[containers[j].ID]
		if obsi != obsj {
			return obsi // obsolete first
		}
		// preserve low container numbers
		if containers[i].Number != containers[j].Number {
			return containers[i].Number > containers[j].Number
		}
		return containers[i].Summary.Created < containers[j].Summary.Created
	})
	slices.Reverse(containers)
}

// reconcileOrphans plans stop + remove for orphaned containers.
func (r *reconciler) reconcileOrphans() {
	for i, oc := range r.observed.Orphans {
		stopNode := r.plan.addNode(Operation{
			Type:       OpStopContainer,
			ResourceID: fmt.Sprintf("orphan:%s", oc.Name),
			Cause:      "orphaned container",
			Container:  &r.observed.Orphans[i].Summary,
			Timeout:    r.options.Timeout,
		}, "")
		r.plan.addNode(Operation{
			Type:       OpRemoveContainer,
			ResourceID: fmt.Sprintf("orphan:%s", oc.Name),
			Cause:      "orphaned container",
			Container:  &r.observed.Orphans[i].Summary,
		}, "", stopNode)
	}
}

// observedSummaries returns the raw container.Summary list for a service,
// needed by nextContainerNumber which expects []container.Summary.
func (r *reconciler) observedSummaries(serviceName string) []container.Summary {
	ocs := r.observed.Containers[serviceName]
	result := make([]container.Summary, len(ocs))
	for i, oc := range ocs {
		result[i] = oc.Summary
	}
	return result
}

// sortedKeys returns the keys of a map sorted alphabetically.
// This ensures deterministic iteration order for reproducible plan output in tests.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
