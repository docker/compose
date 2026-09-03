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
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// providerScenario creates a scenario whose commands can resolve the
// example-provider binary from PATH. The provider echoes options back as env
// variables, which the test service prints with its `env` command — the
// "test-1  | " log prefix anchors the assertions to the container's output.
func providerScenario(t *testing.T, intent string) *Scenario {
	t.Helper()
	provider, err := findExecutable("example-provider")
	if err != nil {
		t.Fatalf("example-provider binary not available (run make example-provider): %v", err)
	}
	s := NewScenario(t, intent)
	s.Env("PATH=" + fmt.Sprintf("%s%s%s", filepath.Dir(provider), string(os.PathListSeparator), os.Getenv("PATH")))
	return s
}

func TestProviderStopHook(t *testing.T) {
	// The example provider writes a sentinel file at PROVIDER_STOP_MARKER when
	// its stop subcommand runs.
	marker := filepath.Join(t.TempDir(), "example-provider-stop-marker")
	providerScenario(t, "stop must invoke the provider binary's stop subcommand").
		Env("PROVIDER_STOP_MARKER="+marker).
		Step("up runs the provider then the service",
			ComposeCmd("up", "-d")).
		Step("stop triggers the provider's stop subcommand",
			ComposeCmd("stop"),
			FileExists(marker))
}

func TestDependsOnMultipleProviders(t *testing.T) {
	providerScenario(t, "a service depending on several providers must receive each provider's variables").
		Step("the service sees both providers' URLs",
			ComposeCmd("up"),
			OutputContains("test-1  | PROVIDER1_URL=https://magic.cloud/provider1"),
			OutputContains("test-1  | PROVIDER2_URL=https://magic.cloud/provider2"))
}

func TestProviderControlChannel(t *testing.T) {
	// The example provider requests its own resolved service config over the
	// stdio control channel and reflects provider.type back through setenv:
	// the dependent service seeing DB_CONFIG_TYPE proves the round trip.
	providerScenario(t, "a provider must be able to request its resolved service config from the running compose process").
		Step("the dependent service sees the value the provider read from its config",
			ComposeCmd("up"),
			OutputContains("test-1  | DB_CONFIG_TYPE=example-provider"))
}

func TestProviderRawSetEnv(t *testing.T) {
	providerScenario(t, "setenv variables must be service-prefixed, rawsetenv injected as-is").
		Step("the service sees both variable flavors",
			ComposeCmd("up"),
			OutputContains("test-1  | SECRETS_URL=https://magic.cloud/secrets"),
			OutputContains("test-1  | CLOUD_REGION=us-east-1"))
}

func TestProviderRawSetEnvOverridesUserEnv(t *testing.T) {
	providerScenario(t, "rawsetenv must override a user-defined variable, with a visible warning").
		Step("the provider's value wins and the override is surfaced",
			ComposeCmd("up"),
			OutputContains("test-1  | CLOUD_REGION=us-east-1"),
			OutputNotContains("test-1  | CLOUD_REGION=user-defined-region"),
			OutputContains("overrides environment variable"))
}

func TestProviderRawSetEnvOverridesInheritedEnv(t *testing.T) {
	providerScenario(t, "rawsetenv must override an inherited passthrough variable, with a visible warning").
		Step("the provider's value wins over the passthrough",
			ComposeCmd("up"),
			OutputContains("test-1  | CLOUD_REGION=us-east-1"),
			OutputContains("overrides environment variable"))
}

func TestProviderRawSetEnvOverridesInheritedEnvMapForm(t *testing.T) {
	providerScenario(t, "rawsetenv must override a map-form passthrough variable, with a visible warning").
		Step("the provider's value wins over the map-form passthrough",
			ComposeCmd("up"),
			OutputContains("test-1  | CLOUD_REGION=us-east-1"),
			OutputContains("overrides environment variable"))
}
