//go:build e2e

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

package e2e

import (
	"strings"
	"testing"
)

func TestIPC(t *testing.T) {
	s := NewScenario(t, "every ipc mode must materialize in the created containers: shareable, service: and container:")
	external := s.Project() + "-external"

	// the container: mode references a container compose doesn't manage;
	// create it up front so its id is known when the steps assert against it
	cid := strings.TrimSpace(s.CLI().RunDockerCmd(t, "run", "-d", "--rm", "--ipc=shareable", "--name", external, "alpine", "top").Stdout())
	s.Defer(DockerCmd("rm", "-f", external).MayFail())

	s.Env("EXTERNAL_NAME="+external).
		Step("up starts the three services",
			ComposeCmd("up", "-d"),
			ServiceState("service", "running"),
			ServiceState("container", "running"),
			ServiceState("shareable", "running")).
		Step("the shareable service owns a shareable ipc namespace",
			DockerCmd("inspect", "-f", "{{.HostConfig.IpcMode}}", s.Project()+"-shareable-1"),
			OutputContains("shareable")).
		Step("the service: mode resolves to the target service's container",
			DockerCmd("inspect", "-f", "{{.HostConfig.IpcMode}}", s.Project()+"-service-1"),
			OutputContains("container:")).
		Step("the container: mode resolves to the external container",
			DockerCmd("inspect", "-f", "{{.HostConfig.IpcMode}}", s.Project()+"-container-1"),
			OutputContains("container:"+cid))
}
