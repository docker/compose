//go:build e2e

/*
Copyright 2026 Docker Compose CLI authors

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
package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/poll"
)

// startLocalRegistry runs a throwaway registry container on a random host
// port for the lifetime of the scenario and returns its host:port address.
func startLocalRegistry(t *testing.T, s *Scenario) string {
	t.Helper()
	c := s.CLI()
	name := s.Project() + "-registry"
	c.RunDockerCmd(t, "run", "--name", name, "-P", "-d", "registry:3")
	s.Defer(DockerCmd("rm", "--force", name))
	port := c.RunDockerCmd(t, "inspect", "--format", `{{ (index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort }}`, name).Stdout()
	registry := "localhost:" + strings.TrimSpace(port)

	registryURL := "http://" + registry + "/v2/"
	poll.WaitOn(t, func(l poll.LogT) poll.Result {
		resp, err := http.Get(registryURL) //nolint:gosec,noctx
		if err != nil {
			return poll.Continue("registry not ready: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 500 {
			return poll.Success()
		}
		return poll.Continue("registry not ready, status %d", resp.StatusCode)
	}, poll.WithTimeout(10*time.Second), poll.WithDelay(500*time.Millisecond))
	return registry
}

func TestOciRemoteUp(t *testing.T) {
	s := NewScenario(t, "up on an oci:// artifact must deploy the published project, bundled env files included")
	registry := startLocalRegistry(t, s)
	ref := registry + "/remote-up:v1"
	s.Env("XDG_CACHE_HOME=" + t.TempDir())
	s.Step("publish pushes the project and its env file to the registry",
		ComposeCmd("publish", "--with-env", "--yes", "--insecure-registry", ref))
	s.FromRemote("oci://"+ref, "--insecure-registry", registry)
	s.Step("up pulls the artifact and starts the service",
		ComposeCmd("up", "-d", "--wait", "--yes").Within(60*time.Second),
		ServiceState("app", "running"),
		// the env file travels as an artifact layer: its effect proves the
		// bundle was consumed whole, not just the compose.yaml
		ContainerEnv("app", "FLAVOR", "published"))
}

func TestOciRemoteTagSelection(t *testing.T) {
	s := NewScenario(t, "the tag of an oci:// reference must select which published revision is deployed")
	registry := startLocalRegistry(t, s)
	refV1 := registry + "/remote-tags:v1"
	refV2 := registry + "/remote-tags:v2"
	s.Env("XDG_CACHE_HOME=" + t.TempDir())
	s.Step("publish the v1 revision",
		ComposeCmd("publish", "--with-env", "--yes", "--insecure-registry", refV1))
	// fixture preparation for the second revision, like a git branch: the
	// anchored copy is edited before publishing under the other tag
	if err := os.WriteFile(filepath.Join(s.Dir(), "app.env"), []byte("FLAVOR=v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Step("publish the v2 revision under another tag",
		ComposeCmd("publish", "--with-env", "--yes", "--insecure-registry", refV2))
	s.FromRemote("oci://"+refV1, "--insecure-registry", registry)
	s.Step("up on the v1 tag deploys the v1 revision, not the latest published one",
		ComposeCmd("up", "-d", "--wait", "--yes").Within(60*time.Second),
		ServiceState("app", "running"),
		ContainerEnv("app", "FLAVOR", "v1"))
}
