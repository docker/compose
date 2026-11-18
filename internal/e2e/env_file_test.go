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

func TestRawEnvFile(t *testing.T) {
	NewScenario(t, "an env_file in raw format must reach the container without interpolation").
		Step("the service prints the raw value, quotes included",
			ComposeCmd("run", "test"),
			OutputContains(`'{"key": "value"}'`))
}

func TestUnusedMissingEnvFile(t *testing.T) {
	const missing = "env file /doesnotexist/.env not found"
	NewScenario(t, "a missing env_file must only break the commands that need the service referencing it").
		Step("up starts the service that does not reference the missing env_file",
			ComposeCmd("up", "-d", "serviceA"),
			ServiceState("serviceA", "running"),
			ServiceNotCreated("serviceB")).
		Step("ps still works",
			ComposeCmd("ps"),
			StdoutContains("serviceA")).
		Step("logs still works",
			ComposeCmd("logs")).
		Step("exec still works",
			ComposeCmd("exec", "serviceA", "echo", "hello"),
			StdoutContains("hello")).
		Step("scale works on a service that does not reference the missing env_file",
			ComposeCmd("scale", "serviceA=2"),
			ServiceScale("serviceA", 2)).
		Step("scaling the service with the missing env_file still fails",
			ComposeCmd("scale", "serviceB=1").MayFail(),
			ExitCode(1),
			StderrContains(missing),
			ServiceNotCreated("serviceB")).
		Step("config --hash works for a service that does not reference it",
			ComposeCmd("config", "--hash", "serviceA")).
		Step("config --hash fails for the service with the missing env_file, as up would",
			ComposeCmd("config", "--hash", "serviceB").MayFail(),
			ExitCode(1),
			StderrContains(missing)).
		Step("config --no-env-resolution skips the env_file resolution",
			ComposeCmd("config", "--no-env-resolution", "--hash", "serviceB")).
		Step("config --hash on the wildcard fails, as it includes that service",
			ComposeCmd("config", "--hash", "*").MayFail(),
			ExitCode(1),
			StderrContains(missing)).
		Step("shell completion still lists the services",
			CompleteCmd("exec", ""),
			StdoutContains("serviceA"),
			StdoutContains("serviceB"))
}

func TestRunEnvFile(t *testing.T) {
	NewScenario(t, "run must resolve the service's env_file relative to the project directory").
		Step("the one-off sees the env_file's variables",
			ComposeCmd("run", "--rm", "serviceC", "env"),
			OutputContains("FOO=BAR"))
}
