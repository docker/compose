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

package compose

import (
	"testing"

	"gotest.tools/v3/assert"
)

// TestProviderBackendWiring pins the adapter between a composeService and
// internal/provider: the provider tests run against their own fake backend,
// so a DryRun that stopped following the service's dry-run flag (providers
// executed for real under --dry-run) or a ProjectLock detached from
// pluginMu (concurrent provider runs no longer serialized) would otherwise
// go unnoticed.
func TestProviderBackendWiring(t *testing.T) {
	s := &composeService{dryRun: true}
	b := providerBackend{relayBackend{s}}
	assert.Assert(t, b.DryRun())
	assert.Assert(t, b.ProjectLock() == &s.pluginMu)

	s.dryRun = false
	assert.Assert(t, !b.DryRun())
}
