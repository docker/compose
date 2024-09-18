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
	"testing"
)

func TestExport(t *testing.T) {
	const projectName = "e2e-export-service"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/export/compose.yaml", "--project-name", projectName, "up", "-d", "service")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "export", "-o", "service.tar", "service")
}

func TestExportWithReplicas(t *testing.T) {
	const projectName = "e2e-export-service-with-replicas"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/export/compose.yaml", "--project-name", projectName, "up", "-d", "service-with-replicas")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "export", "-o", "r1.tar", "--index=1", "service-with-replicas")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "export", "-o", "r2.tar", "--index=2", "service-with-replicas")
}
