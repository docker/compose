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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
)

// defaultRelayImage is the published network-relay image deployed in place of
// a provider service that published endpoints. Overridable for development
// and air-gapped setups with COMPOSE_RELAY_IMAGE.
const defaultRelayImage = "docker/compose-relay:v1"

func relayImage() string {
	if img := os.Getenv("COMPOSE_RELAY_IMAGE"); img != "" {
		return img
	}
	return defaultRelayImage
}

// relayRoutesSpec renders endpoints as the relay's RELAY_ROUTES value,
// canonically ordered so it doubles as the identity the relay label hashes.
// Upstreams are rewritten for the relay's vantage point (relayUpstream)
// before rendering, so the identity follows what the relay actually dials.
func relayRoutesSpec(endpoints map[int]string) string {
	ports := make([]int, 0, len(endpoints))
	for port := range endpoints {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	routes := make([]string, 0, len(ports))
	for _, port := range ports {
		routes = append(routes, fmt.Sprintf("%d=%s", port, relayUpstream(endpoints[port])))
	}
	return strings.Join(routes, ",")
}

// relayUpstream rewrites a host-relative upstream for the relay's vantage
// point. Providers express endpoints from the host's perspective — a loopback
// or unspecified address names the machine compose runs on — but the relay
// dials from its own network namespace, where those addresses name the relay
// container itself. host.docker.internal resolves natively on Docker Desktop
// and is provisioned through ExtraHosts (host-gateway) on plain Linux
// engines. Anything else (a LAN IP, a DNS name) is reachable as-is from the
// relay and passes verbatim.
func relayUpstream(endpoint string) string {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		// validated at parse time (parseEndpointMessage); keep verbatim
		return endpoint
	}
	hostRelative := host == "" || strings.EqualFold(host, "localhost")
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		hostRelative = true
	}
	if !hostRelative {
		return endpoint
	}
	return net.JoinHostPort("host.docker.internal", port)
}

func relayIdentity(routes string) string {
	digest := sha256.Sum256([]byte(relayImage() + "|" + routes))
	return hex.EncodeToString(digest[:])[:12]
}

// relayNetworks returns the compose network keys the relay must join: the
// union of the networks of every service depending on the provider service —
// the consumers the relay exists for — falling back to the project default.
func relayNetworks(project *types.Project, service types.ServiceConfig) []string {
	set := map[string]bool{}
	for _, s := range project.Services {
		if _, ok := s.DependsOn[service.Name]; !ok {
			continue
		}
		for key := range s.Networks {
			// a resolved project declares every service network, but guard
			// anyway: an unknown key would yield an empty network name later
			if _, ok := project.Networks[key]; ok {
				set[key] = true
			}
		}
	}
	if len(set) == 0 {
		if _, ok := project.Networks["default"]; ok {
			set["default"] = true
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// relayInfoAnswer is the get-relay-info reply: the network the relay
// reaches the provider's own runtime through, with the address the HOST owns
// on it — its gateway. A provider running its service locally can bind that
// address: reachable from the relay (same-bridge local delivery), and —
// unlike a project's own bridge networks, which every service on them can
// also reach — joined by nothing else. A single entry, or none when it
// could not be resolved (see ensureRelayLinkNetwork): a provider must treat
// that as "bind elsewhere".
type relayInfoAnswer struct {
	Networks []relayNetworkInfo `json:"networks"`
}

type relayNetworkInfo struct {
	// Name is the concrete engine-level network name.
	Name string `json:"name"`
	// Gateway is the address the provider's host owns that the relay can
	// reach on this network: the dedicated relay-link network's IPv4
	// gateway on a standalone engine, the host's own loopback under Docker
	// Desktop — whose proxy dials host-process endpoints through
	// 127.0.0.1, making it factually the gateway to the host from the
	// relay's vantage point. Empty when unresolved (network not created
	// yet, driver without a host-owned gateway, IPv6-only).
	Gateway string `json:"gateway,omitempty"`
}

// relayInfo assembles the get-relay-info answer for one provider service:
// the dedicated relay-link network (ensureRelayLinkNetwork) — never one of
// the project's own bridge networks, which every other service attached to
// them could also reach — resolved for the address a locally-run endpoint
// should bind. Compose owns the platform knowledge — the provider just
// binds what is announced. Best-effort by design: a provider must treat a
// missing gateway as "bind elsewhere".
func (s *composeService) relayInfo(ctx context.Context, project *types.Project, service types.ServiceConfig) relayInfoAnswer {
	answer := relayInfoAnswer{Networks: []relayNetworkInfo{}}
	// Under Docker Desktop the networks (and their gateways) live inside
	// the VM: unreachable AND unbindable from the provider's host, and a
	// dedicated bridge network would be no more exclusive than any other —
	// every network's gateway is, from the host's side, just the Desktop
	// proxy's own loopback. The address a host process binds to be reached
	// from the relay is that loopback, so that is what gets announced;
	// there is nothing to create. Detection errors fall through to the
	// dedicated-network path — best-effort.
	if desktopActive, _ := s.isDesktopIntegrationActive(ctx); desktopActive {
		answer.Networks = append(answer.Networks, relayNetworkInfo{Name: "desktop", Gateway: "127.0.0.1"})
		return answer
	}
	name, gateway, err := s.ensureRelayLinkNetwork(ctx, project, service)
	if err != nil {
		logrus.Warnf("relay link network for service %q: %v", service.Name, err)
		return answer
	}
	if name == "" {
		// removed concurrently right after being created (see
		// ensureRelayLinkNetwork): best-effort, no entry to give — the
		// provider's next get-relay-info retries the whole thing
		return answer
	}
	answer.Networks = append(answer.Networks, relayNetworkInfo{Name: name, Gateway: gateway})
	return answer
}

// relayLinkNetworkName is the deterministic name of the dedicated network
// created for one provider service's relay link — the sole channel between
// the relay container and the provider's own runtime. Never a project's
// user-declared network: distinguishing it structurally rules out any name
// collision with one, and keeps its lifecycle independent of the project's
// own declared topology (it lives and dies with the service's relay, not
// with `up`/`down` of the whole project).
func relayLinkNetworkName(projectName, serviceName string) string {
	return fmt.Sprintf("%s_%s_relay", projectName, serviceName)
}

// findRelayLinkNetwork looks up a service's dedicated relay link network
// without creating it: get-relay-info (ensureRelayLinkNetwork, below) is the
// only place that ever creates one, because sending that message is itself
// the provider's declaration that it binds locally and needs the address —
// docs/extension.md: "Only meaningful for a provider running its service
// locally; a provider backing the service with a remote resource never
// needs it." A provider that publishes an endpoint without ever asking
// (a remote resource, e.g. an RDS instance) must never get one conjured
// for it just because it happened to publish something.
func (s *composeService) findRelayLinkNetwork(ctx context.Context, projectName, serviceName string) (name string, ok bool, err error) {
	name = relayLinkNetworkName(projectName, serviceName)
	filters := projectFilter(projectName).Add("label", serviceFilter(serviceName)).Add("label", api.RelayNetworkLabel)
	existing, err := s.apiClient().NetworkList(ctx, client.NetworkListOptions{Filters: filters})
	if err != nil {
		return "", false, fmt.Errorf("list relay link network for service %s: %w", serviceName, err)
	}
	return name, len(existing.Items) > 0, nil
}

// ensureRelayLinkNetwork converges the dedicated bridge network one provider
// service's relay link binds to: created on first get-relay-info request,
// reused across every later one, and carrying no traffic other than the
// relay reaching the provider's runtime. Unlike relayNetworks (the
// dependents' networks the relay joins to expose the compose-native
// alias), nothing else is ever attached to this one — not a dependent, not
// a sibling project container — so the answer has exactly one,
// unambiguous, exclusive gateway to give.
//
// Called only from relayInfo, in response to the provider's own
// get-relay-info request — see findRelayLinkNetwork for why creation must
// stay gated on that signal, not on endpoints merely being published.
//
// internal: true — the network never needs, or gets, outbound connectivity;
// its only job is carrying the relay's own traffic to the address the
// provider binds. Docker still assigns it a host-owned gateway address like
// any other bridge network regardless of the internal flag.
func (s *composeService) ensureRelayLinkNetwork(ctx context.Context, project *types.Project, service types.ServiceConfig) (name string, gateway string, err error) {
	name, ok, err := s.findRelayLinkNetwork(ctx, project.Name, service.Name)
	if err != nil {
		return "", "", err
	}
	if ok {
		gw, err := s.relayLinkNetworkGateway(ctx, name)
		switch {
		case err == nil:
			return name, gw, nil
		case errdefs.IsNotFound(err):
			// removed concurrently between the list above and this inspect
			// (e.g. a same-service up that just decided to stop publishing):
			// fall through to create it, same as if it had never existed
		default:
			return "", "", err
		}
	}

	if _, err := s.apiClient().NetworkCreate(ctx, name, client.NetworkCreateOptions{
		Labels: map[string]string{
			api.ProjectLabel:      project.Name,
			api.ServiceLabel:      service.Name,
			api.RelayNetworkLabel: "true",
		},
		Driver:   "bridge",
		Internal: true,
	}); err != nil {
		if !errdefs.IsConflict(err) {
			return "", "", fmt.Errorf("create relay link network for service %s: %w", service.Name, err)
		}
		// a concurrent up for the same service creating it first is not a
		// failure — same tolerance createNetwork already has for a
		// project's own declared networks. But the deterministic name can
		// also collide with an unrelated, unlabeled network (e.g. a user
		// declaring networks: {<service>_relay: {}}): re-check by label
		// before inspecting by name, so a name clash is never mistaken for
		// the relay's own network and adopted into the isolation boundary.
		if _, ok, err := s.findRelayLinkNetwork(ctx, project.Name, service.Name); err != nil {
			return "", "", err
		} else if !ok {
			return "", "", fmt.Errorf("create relay link network for service %s: a network named %q already exists and is not a relay link network", service.Name, name)
		}
	}

	gw, err := s.relayLinkNetworkGateway(ctx, name)
	switch {
	case err == nil:
		return name, gw, nil
	case errdefs.IsNotFound(err):
		// removed concurrently between the create (won or lost to a
		// conflict) and this inspect, e.g. a concurrent down: best-effort,
		// same as the ok+NotFound case above — the provider's next
		// get-relay-info retries the whole thing
		return "", "", nil
	default:
		return "", "", err
	}
}

func (s *composeService) relayLinkNetworkGateway(ctx context.Context, idOrName string) (string, error) {
	inspected, err := s.apiClient().NetworkInspect(ctx, idOrName, client.NetworkInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspect relay link network %s: %w", idOrName, err)
	}
	for _, cfg := range inspected.Network.IPAM.Config {
		if cfg.Gateway.IsValid() && cfg.Gateway.Is4() {
			return cfg.Gateway.String(), nil
		}
	}
	return "", nil
}

// removeRelayLinkNetwork removes a service's relay link network, if any —
// the counterpart to ensureRelayLinkNetwork, called wherever the service's
// relay itself is torn down (see removeServiceRelay) so the network never
// outlives the relay it exists for. Every caller removes the relay container
// first, so the network is not expected to still have endpoints attached —
// but the daemon disconnects them asynchronously, so a ContainerRemove that
// already returned can still race NetworkRemove here; this tolerates that
// (errdefs.IsConflict) the same way ensureRelayLinkNetworksDown does for the
// down path, besides the network already being gone (errdefs.IsNotFound).
func (s *composeService) removeRelayLinkNetwork(ctx context.Context, projectName, serviceName string) error {
	filters := projectFilter(projectName).Add("label", serviceFilter(serviceName)).Add("label", api.RelayNetworkLabel)
	existing, err := s.apiClient().NetworkList(ctx, client.NetworkListOptions{Filters: filters})
	if err != nil {
		return fmt.Errorf("list relay link network for service %s: %w", serviceName, err)
	}
	for _, n := range existing.Items {
		if _, err := s.apiClient().NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{}); err != nil {
			if errdefs.IsNotFound(err) {
				continue
			}
			if errdefs.IsConflict(err) {
				logrus.Warnf("relay link network %s still has active endpoints, skipping removal", n.Name)
				continue
			}
			return fmt.Errorf("remove relay link network for service %s: %w", serviceName, err)
		}
	}
	return nil
}

// ensureServiceRelay converges the relay container standing in for a provider
// service that published endpoints: consumers reach the provider's resource
// at the compose-native address (http://<service>:<port>) through it. The
// relay is a regular project container (standard compose labels, canonical
// name, service alias on the consumers' networks) so label-driven commands
// treat it as the service, plus the RelayLabel identifying its role — the
// reconciler leaves provider services' containers alone, and process-level
// commands (exec) refuse it.
//
// networkKeys (see relayNetworks) is computed by the caller under the shared
// project mutex: this function performs only Docker API work and must not
// touch project.Services, which concurrent provider runs mutate.
func (s *composeService) ensureServiceRelay(ctx context.Context, project *types.Project, service types.ServiceConfig, endpoints map[int]string, networkKeys []string) error {
	routes := relayRoutesSpec(endpoints)
	identity := relayIdentity(routes)
	name := getContainerName(project.Name, service, 1)

	if len(networkKeys) == 0 {
		logrus.Warnf("service %q published endpoints but no service depends on it and the project has no default network; skipping relay", service.Name)
		// a relay from a previous up (dependents have since dropped to zero)
		// must not linger with stale network attachments, and an earlier
		// get-relay-info in this same up may have speculatively created the
		// link network before this outcome was known — removeServiceRelay
		// clears both.
		return s.removeServiceRelay(ctx, project.Name, service.Name)
	}

	// Only FOUND, never created here: a dedicated network exists only when
	// the provider itself asked for one via get-relay-info (relayInfo owns
	// creation — see findRelayLinkNetwork), which is the provider's own
	// declaration that it binds locally. A provider backing the service
	// with a remote resource (an RDS instance, say) can publish an endpoint
	// without ever asking, and must get no network conjured for it just
	// because it did — nothing on its side would ever use one.
	linkNetwork := ""
	if linkName, ok, err := s.findRelayLinkNetwork(ctx, project.Name, service.Name); err != nil {
		return err
	} else if ok {
		linkNetwork = linkName
	}

	existing, err := s.findRelayContainer(ctx, project.Name, service.Name)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.Labels[api.RelayLabel] == identity {
			// The identity only covers image+routes: a dependent service
			// added on a new network after the relay is already up must
			// still be connected, whether or not anything else changed.
			if err := s.ensureRelayNetworks(ctx, project, existing, service, networkKeys, linkNetwork); err != nil {
				return err
			}
			switch existing.State {
			case container.StateRunning, container.StateRestarting:
				// Up to date; restarting means Docker is already recovering
				// it — recreating would tear down in-flight connections.
				return nil
			case container.StateCreated, container.StateExited:
				// Routes unchanged: start the existing container rather than
				// recreating it.
				if _, err := s.apiClient().ContainerStart(ctx, existing.ID, client.ContainerStartOptions{}); err != nil {
					return fmt.Errorf("start relay for service %s: %w", service.Name, err)
				}
				return nil
			case container.StatePaused:
				if _, err := s.apiClient().ContainerUnpause(ctx, existing.ID, client.ContainerUnpauseOptions{}); err != nil {
					return fmt.Errorf("unpause relay for service %s: %w", service.Name, err)
				}
				return nil
			}
		}
		if existing.State == container.StateRemoving {
			// the daemon is already removing it: a concurrent ContainerRemove
			// fails with "removal already in progress", so wait for the name
			// to free up instead
			if err := s.waitRelayRemoved(ctx, project.Name, service.Name); err != nil {
				return err
			}
		} else if _, err := s.apiClient().ContainerRemove(ctx, existing.ID, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove stale relay for service %s: %w", service.Name, err)
		}
	}

	s.events.On(creatingEvent("Relay " + name))
	id, err := s.createRelayContainer(ctx, project, service, name, routes, identity, networkKeys, linkNetwork)
	if err != nil {
		return err
	}
	if _, err := s.apiClient().ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("start relay for service %s: %w", service.Name, err)
	}
	s.events.On(createdEvent("Relay " + name))
	return nil
}

// ensureRelayNetworks connects an already up-to-date relay to any network in
// networkKeys it isn't attached to yet, plus linkNetwork (empty under
// Desktop — see ensureServiceRelay) without a service alias: nothing
// addresses the relay by name there, it exists purely so the relay can
// reach the provider. relayIdentity hashes image+routes only, not network
// topology, so a service added later on a new network leaves the relay's
// identity — and so the reuse decision in ensureServiceRelay — unchanged;
// without this, the relay would silently stay unreachable from that
// network's consumers (or, for linkNetwork, from the provider itself).
func (s *composeService) ensureRelayNetworks(ctx context.Context, project *types.Project, existing *container.Summary, service types.ServiceConfig, networkKeys []string, linkNetwork string) error {
	connected := map[string]bool{}
	if existing.NetworkSettings != nil {
		for name := range existing.NetworkSettings.Networks {
			connected[name] = true
		}
	}
	for _, key := range networkKeys {
		netName := project.Networks[key].Name
		if connected[netName] {
			continue
		}
		if _, err := s.apiClient().NetworkConnect(ctx, netName, client.NetworkConnectOptions{
			Container:      existing.ID,
			EndpointConfig: &network.EndpointSettings{Aliases: []string{service.Name}},
		}); err != nil && !errdefs.IsConflict(err) {
			// a concurrent ensureServiceRelay run for another provider service
			// may have connected it to this same network in the window since
			// our ContainerList snapshot; the daemon's "endpoint already
			// exists" is the desired state, not a failure
			return fmt.Errorf("connect relay for service %s to network %s: %w", service.Name, netName, err)
		}
	}
	if linkNetwork != "" && !connected[linkNetwork] {
		if _, err := s.apiClient().NetworkConnect(ctx, linkNetwork, client.NetworkConnectOptions{
			Container: existing.ID,
		}); err != nil && !errdefs.IsConflict(err) {
			return fmt.Errorf("connect relay for service %s to its relay link network: %w", service.Name, err)
		}
	}
	return nil
}

// ensureRelayLinkNetworksDown returns down ops removing every provider
// service's relay link network — down.go's counterpart to
// ensureRelayLinkNetwork: removeServiceRelay's own network cleanup only
// runs when a later `up` decides a relay is no longer needed, a path a
// full `down` never takes (the relay container itself is swept by the
// generic per-service container removal instead), so without this the
// dedicated network would outlive the relay it was created for.
// Looked up directly by label — not by walking project.Services and
// checking Provider — because a project reconstructed from live containers
// (getProjectWithResources, the path a `down` without an explicit compose
// file takes) never repopulates Provider: nothing in a container's own
// labels says its service declared one, so a per-service check here would
// silently skip every service and leak the network on every such down —
// the common case, not an edge one.
func (s *composeService) ensureRelayLinkNetworksDown(ctx context.Context, project *types.Project) []downOp {
	return []downOp{func() error {
		filters := projectFilter(project.Name).Add("label", api.RelayNetworkLabel)
		networks, err := s.apiClient().NetworkList(ctx, client.NetworkListOptions{Filters: filters})
		if err != nil {
			return fmt.Errorf("list relay link networks for project %s: %w", project.Name, err)
		}
		var errs []error
		for _, n := range networks.Items {
			// one project can have several provider services, each with its
			// own relay-link network: a transient inspect/remove failure on
			// one must not leave the rest unprocessed, unlike a single
			// return would.
			inspected, err := s.apiClient().NetworkInspect(ctx, n.ID, client.NetworkInspectOptions{})
			if errdefs.IsNotFound(err) {
				continue
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("inspect relay link network %s: %w", n.Name, err))
				continue
			}
			if len(inspected.Network.Containers) > 0 {
				// the daemon's async disconnect of the just-removed relay
				// container hasn't caught up yet; a later down retries
				logrus.Warnf("relay link network %s is still in use, skipping removal", n.Name)
				continue
			}
			if _, err := s.apiClient().NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("remove relay link network %s: %w", n.Name, err))
			}
		}
		return errors.Join(errs...)
	}}
}

// waitRelayRemoved polls until the service's relay container is gone, giving
// an in-progress daemon-side removal time to release the container's name.
func (s *composeService) waitRelayRemoved(ctx context.Context, projectName, serviceName string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		existing, err := s.findRelayContainer(ctx, projectName, serviceName)
		if err != nil {
			return err
		}
		if existing == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("relay for service %s is stuck being removed", serviceName)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// removeServiceRelay removes a service's relay container, if any. Called
// whenever an up finds the provider publishing no endpoint — whether it
// never did, or a relay from an earlier up is now stale — so a relay that
// still routes to an upstream the provider no longer serves doesn't linger.
func (s *composeService) removeServiceRelay(ctx context.Context, projectName, serviceName string) error {
	existing, err := s.findRelayContainer(ctx, projectName, serviceName)
	if err != nil {
		return err
	}
	if existing != nil {
		eventID := "Relay " + getCanonicalContainerName(*existing)
		s.events.On(removingEvent(eventID))
		if existing.State == container.StateRemoving {
			// the daemon is already removing it: a concurrent ContainerRemove
			// fails with "removal already in progress", so wait for the name
			// to free up instead
			if err := s.waitRelayRemoved(ctx, projectName, serviceName); err != nil {
				return err
			}
		} else if _, err := s.apiClient().ContainerRemove(ctx, existing.ID, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove stale relay for service %s: %w", serviceName, err)
		}
		s.events.On(removedEvent(eventID))
	}
	// Attempted even when no relay container exists: get-relay-info can
	// create the link network speculatively (a provider may ask before
	// deciding whether to publish an endpoint), leaving it orphaned if the
	// relay itself never got deployed.
	if err := s.removeRelayLinkNetwork(ctx, projectName, serviceName); err != nil {
		return err
	}
	return nil
}

// findRelayContainer returns the service's relay container, if any.
func (s *composeService) findRelayContainer(ctx context.Context, projectName, serviceName string) (*container.Summary, error) {
	f := projectFilter(projectName)
	f.Add("label", serviceFilter(serviceName))
	f.Add("label", api.RelayLabel)
	result, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, nil
	}
	return &result.Items[0], nil
}

func (s *composeService) createRelayContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	name, routes, identity string, networkKeys []string, linkNetwork string,
) (string, error) {
	labels := types.Labels{
		api.ProjectLabel:         project.Name,
		api.ServiceLabel:         service.Name,
		api.VersionLabel:         api.ComposeVersion,
		api.ConfigFilesLabel:     strings.Join(project.ComposeFiles, ","),
		api.WorkingDirLabel:      project.WorkingDir,
		api.ContainerNumberLabel: "1",
		api.OneoffLabel:          "False",
		// Label-driven commands filter on ConfigHashLabel presence: without
		// it the relay would be invisible to ps/stop/exec run without the
		// compose file. The relay identity doubles as its config hash.
		api.ConfigHashLabel: identity,
		api.RelayLabel:      identity,
	}

	config := &container.Config{
		Image:  relayImage(),
		Env:    []string{"RELAY_ROUTES=" + routes},
		Labels: labels,
	}
	hostConfig := &container.HostConfig{
		// host.docker.internal resolves natively on Docker Desktop; the
		// host-gateway mapping makes the same upstream host name work on a
		// plain Linux engine.
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		// The relay only dials out and forwards bytes: it needs none of
		// Docker's default capabilities. NET_BIND_SERVICE is kept because a
		// route commonly targets a privileged port (e.g. 80, 443) that the
		// relay — running unprivileged as UID 65532 — must still be able to
		// listen on inside its own container.
		CapDrop: []string{"ALL"},
		CapAdd:  []string{"NET_BIND_SERVICE"},
	}

	// First network at creation, remaining ones connected afterwards — the
	// engine accepts a single endpoint in the create payload.
	endpointSettings := func(_ string) *network.EndpointSettings {
		return &network.EndpointSettings{Aliases: []string{service.Name}}
	}
	first := project.Networks[networkKeys[0]].Name
	networking := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			first: endpointSettings(first),
		},
	}

	created, err := s.apiClient().ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:             name,
		Config:           config,
		HostConfig:       hostConfig,
		NetworkingConfig: networking,
	})
	if errdefs.IsNotFound(err) {
		if err := s.pullRelayImage(ctx); err != nil {
			return "", err
		}
		created, err = s.apiClient().ContainerCreate(ctx, client.ContainerCreateOptions{
			Name:             name,
			Config:           config,
			HostConfig:       hostConfig,
			NetworkingConfig: networking,
		})
	}
	if err != nil {
		return "", fmt.Errorf("create relay for service %s: %w", service.Name, err)
	}

	for _, key := range networkKeys[1:] {
		netName := project.Networks[key].Name
		if _, err := s.apiClient().NetworkConnect(ctx, netName, client.NetworkConnectOptions{
			Container:      created.ID,
			EndpointConfig: endpointSettings(netName),
		}); err != nil {
			// remove the half-connected container: left in place (with its
			// restart policy) it would serve only a subset of the consumers'
			// networks, and its identity would shield it from recreation
			if _, rmErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
				logrus.Warnf("removing half-connected relay %s: %v", name, rmErr)
			}
			return "", fmt.Errorf("connect relay for service %s to network %s: %w", service.Name, netName, err)
		}
	}
	if linkNetwork != "" {
		// no alias: nothing addresses the relay by name on this network, it
		// exists purely so the relay can reach the provider (see
		// ensureRelayLinkNetwork).
		if _, err := s.apiClient().NetworkConnect(ctx, linkNetwork, client.NetworkConnectOptions{Container: created.ID}); err != nil {
			if _, rmErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
				logrus.Warnf("removing half-connected relay %s: %v", name, rmErr)
			}
			return "", fmt.Errorf("connect relay for service %s to its relay link network: %w", service.Name, err)
		}
	}
	return created.ID, nil
}

func (s *composeService) pullRelayImage(ctx context.Context) error {
	image := relayImage()
	s.events.On(newEvent(image, api.Working, "Pulling"))
	response, err := s.apiClient().ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", image, err)
	}
	defer func() { _ = response.Close() }()
	if err := response.Wait(ctx); err != nil {
		return fmt.Errorf("pull %s: %w", image, err)
	}
	s.events.On(newEvent(image, api.Done, "Pulled"))
	return nil
}

// isRelayContainer reports whether a container is a service's network relay
// rather than a regular instance of it: a static scratch-based binary with
// no shell or service process, so anything expecting one to be there —
// secrets/configs injection, lifecycle hooks, process-level commands — must
// skip it.
func isRelayContainer(ctr container.Summary) bool {
	return isRelay(ctr.Labels)
}

// checkRelayTarget refuses process-level operations on a relay container: it
// stands in for the provider's resource on the network, but there is no
// service process in it to act on.
func checkRelayTarget(target container.Summary, serviceName, operation string) error {
	if !isRelayContainer(target) {
		return nil
	}
	return fmt.Errorf("service %q is managed by a provider: its container is a network relay and does not support %s", serviceName, operation)
}
