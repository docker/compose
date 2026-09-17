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

	existing, err := s.findRelayContainer(ctx, project.Name, service.Name)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.Labels[api.RelayLabel] == identity {
			// The identity only covers image+routes: a dependent service
			// added on a new network after the relay is already up must
			// still be connected, whether or not anything else changed.
			if err := s.ensureRelayNetworks(ctx, project, existing, service, networkKeys); err != nil {
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

	if len(networkKeys) == 0 {
		logrus.Warnf("service %q published endpoints but no service depends on it and the project has no default network; skipping relay", service.Name)
		return nil
	}

	s.events.On(creatingEvent("Relay " + name))
	id, err := s.createRelayContainer(ctx, project, service, name, routes, identity, networkKeys)
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
// networkKeys it isn't attached to yet. relayIdentity hashes image+routes
// only, not network topology, so a service added later on a new network
// leaves the relay's identity — and so the reuse decision in
// ensureServiceRelay — unchanged; without this, the relay would silently
// stay unreachable from that network's consumers.
func (s *composeService) ensureRelayNetworks(ctx context.Context, project *types.Project, existing *container.Summary, service types.ServiceConfig, networkKeys []string) error {
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
	return nil
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
	if existing == nil {
		return nil
	}
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

func (s *composeService) createRelayContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, name, routes, identity string, networkKeys []string) (string, error) {
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
