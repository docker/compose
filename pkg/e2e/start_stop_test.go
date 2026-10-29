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

import "testing"

func TestStartStop(t *testing.T) {
	s := NewScenario(t, "stop must halt the project's containers in place, start must bring the same ones back")
	s.Step("up starts every service",
		ComposeCmd("up", "-d"),
		ServiceState("simple", "running"),
		ServiceState("another", "running")).
		Step("ls reports the project as running",
			ComposeCmd("ls", "--format", "json"),
			OutputContains(`"Name":"`+s.Project()+`"`)).
		Step("stop halts the containers without removing them",
			ComposeCmd("stop"),
			ServiceState("simple", "exited"),
			ServiceState("another", "exited")).
		Step("a stopped project is hidden from ls",
			ComposeCmd("ls", "--format", "json"),
			OutputNotContains(`"Name":"`+s.Project()+`"`)).
		Step("ls --all still lists the stopped project",
			ComposeCmd("ls", "--all", "--format", "json"),
			OutputContains(`"Name":"`+s.Project()+`"`)).
		Step("start brings the same containers back",
			ComposeCmd("start"),
			ServiceState("simple", "running"),
			ServiceState("another", "running"),
			NotRecreated("simple", "another"))
}

func TestStartStopWithDependencies(t *testing.T) {
	NewScenario(t, "stop must only halt the requested service, start must also start its dependencies").
		Step("up starts the service and its dependency",
			ComposeCmd("up", "-d"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running")).
		Step("stop foo leaves the dependency running",
			ComposeCmd("stop", "foo"),
			ServiceState("foo", "exited"),
			ServiceState("bar", "running")).
		Step("stop halts the whole project",
			ComposeCmd("stop"),
			ServiceState("foo", "exited"),
			ServiceState("bar", "exited")).
		Step("start foo brings its dependency back too",
			ComposeCmd("start", "foo"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running"),
			NotRecreated("foo", "bar"))
}

func TestUpNoDeps(t *testing.T) {
	NewScenario(t, "up --no-deps must not create the service's dependencies").
		Step("up --no-deps starts only the requested service",
			ComposeCmd("up", "--no-deps", "-d", "foo"),
			ServiceState("foo", "running"),
			ServiceNotCreated("bar"))
}

func TestStartStopWithOneOffs(t *testing.T) {
	NewScenario(t, "stop, start and restart must act on services and leave one-off containers untouched").
		Step("up starts the services",
			ComposeCmd("up", "-d"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running")).
		Step("run adds a one-off container alongside the service",
			ComposeCmd("run", "-d", "bar", "sleep", "infinity"),
			OneOffState("bar", "running"),
			NotRecreated("foo", "bar")).
		Step("stop halts the services but not the one-off",
			ComposeCmd("stop"),
			ServiceState("foo", "exited"),
			ServiceState("bar", "exited"),
			OneOffState("bar", "running"),
			OneOffsUntouched("bar")).
		Step("start resumes the services without touching the one-off",
			ComposeCmd("start"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running"),
			OneOffsUntouched("bar")).
		Step("restart restarts the services but not the one-off",
			ComposeCmd("restart"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running"),
			NotRecreated("foo", "bar"),
			OneOffsUntouched("bar")).
		Step("down --remove-orphans removes the one-off as well",
			ComposeCmd("down", "--remove-orphans"),
			ServiceNotCreated("foo"),
			ServiceNotCreated("bar"),
			OneOffsRemoved("bar"))
}

func TestStartAlreadyRunning(t *testing.T) {
	NewScenario(t, "start on an already running service must be a silent no-op").
		Step("up starts the service and waits for it",
			ComposeCmd("up", "-d", "--wait"),
			ServiceState("simple", "running")).
		Step("start leaves the running container alone and writes nothing to stdout",
			ComposeCmd("start", "simple"),
			StdoutEmpty(),
			ServiceState("simple", "running"),
			NotRecreated("simple"))
}

func TestStopAlreadyStopped(t *testing.T) {
	s := NewScenario(t, "stop on an already stopped service must succeed and leave it stopped")
	s.Step("up starts the services and waits for them",
		ComposeCmd("up", "-d", "--wait"),
		ServiceState("simple", "running")).
		Step("stop halts the service",
			ComposeCmd("stop", "simple"),
			ServiceState("simple", "exited")).
		Step("stopping it again succeeds and leaves it stopped",
			ComposeCmd("stop", "simple"),
			ServiceState("simple", "exited"),
			NotRecreated("simple"),
			// TODO: for consistency, this should NOT write any output because the
			// 		container is already stopped
			StderrContains("Container "+s.Project()+"-simple-1 Stopped"))
}

func TestStartStopMultipleServices(t *testing.T) {
	NewScenario(t, "stop and start must act on every service named on the command line").
		Step("up starts the services and waits for them",
			ComposeCmd("up", "-d", "--wait"),
			ServiceState("simple", "running"),
			ServiceState("another", "running")).
		Step("stop halts both named services",
			ComposeCmd("stop", "simple", "another"),
			ServiceState("simple", "exited"),
			ServiceState("another", "exited")).
		Step("start brings both named services back, reusing their containers",
			ComposeCmd("start", "simple", "another"),
			ServiceState("simple", "running"),
			ServiceState("another", "running"),
			NotRecreated("simple", "another"))
}

func TestStartSingleServiceAndDependency(t *testing.T) {
	NewScenario(t, "start of a created service must start its dependency chain and nothing else").
		Step("create prepares the desired service and its dependencies only",
			ComposeCmd("create", "desired"),
			ServiceState("desired", "created"),
			ServiceState("dep_1", "created"),
			ServiceState("dep_2", "created"),
			ServiceNotCreated("another"),
			ServiceNotCreated("another_2")).
		Step("start brings up the desired service with its dependencies, nothing else",
			ComposeCmd("start", "desired"),
			ServiceState("desired", "running"),
			ServiceState("dep_1", "running"),
			ServiceState("dep_2", "running"),
			ServiceNotCreated("another"),
			ServiceNotCreated("another_2"))
}

func TestStartStopMultipleFiles(t *testing.T) {
	s := NewScenario(t, "stop must only act on the services of the compose file it is given, in a project spanning several files")
	s.Step("up starts the services of the first file",
		ComposeCmd("up", "-d"),
		ServiceState("simple", "running"),
		ServiceState("another", "running"))
	s.FromFile("other.yaml").
		Step("up of a second file adds its services to the same project",
			ComposeCmd("up", "-d"),
			ServiceState("simple", "running"),
			ServiceState("another", "running"),
			ServiceState("a-different-one", "running"),
			ServiceState("and-another-one", "running"))
	s.FromFile("compose.yaml").
		Step("stop with the first file halts its services only",
			ComposeCmd("stop"),
			ServiceState("simple", "exited"),
			ServiceState("another", "exited"),
			ServiceState("a-different-one", "running"),
			ServiceState("and-another-one", "running"))
}
