//go:build e2e

/*
   Copyright 2023 Docker Compose CLI authors

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
	"path/filepath"
	"testing"
)

func TestExport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "service.tar")
	NewScenario(t, "export must write the service container's filesystem as a tar archive").
		Step("up starts the service",
			ComposeCmd("up", "-d", "service"),
			ServiceState("service", "running")).
		Step("export writes the container filesystem to the output file",
			ComposeCmd("export", "-o", out, "service"),
			FileExists(out))
}

func TestExportWithReplicas(t *testing.T) {
	dir := t.TempDir()
	r1, r2 := filepath.Join(dir, "r1.tar"), filepath.Join(dir, "r2.tar")
	NewScenario(t, "export --index must pick the requested replica of a scaled service").
		Step("up starts the replicas",
			ComposeCmd("up", "-d", "service-with-replicas"),
			ServiceScale("service-with-replicas", 3)).
		Step("export --index=1 writes the first replica's filesystem",
			ComposeCmd("export", "-o", r1, "--index=1", "service-with-replicas"),
			FileExists(r1)).
		Step("export --index=2 writes the second replica's filesystem",
			ComposeCmd("export", "-o", r2, "--index=2", "service-with-replicas"),
			FileExists(r2))
}
