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

package provider

import (
	"context"
	"os/exec"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/internal/desktop"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// testBackend serves a mocked Docker client, with events nobody listens to.
type testBackend struct {
	apiClient client.APIClient
	cli       command.Cli
	dryRun    bool
	mu        sync.Mutex
}

func (b *testBackend) APIClient() client.APIClient { return b.apiClient }
func (b *testBackend) Events() api.EventProcessor  { return progress.Noop{} }
func (b *testBackend) CLI() command.Cli            { return b.cli }
func (b *testBackend) DryRun() bool                { return b.dryRun }
func (b *testBackend) ProjectLock() sync.Locker    { return &b.mu }

func (b *testBackend) IsDesktopIntegrationActive(ctx context.Context) (bool, error) {
	endpoint, err := desktop.Endpoint(ctx, b.apiClient)
	return endpoint != "", err
}

func (b *testBackend) PrepareShellOut(_ context.Context, env types.Mapping, cmd *exec.Cmd) error {
	cmd.Env = env.Values()
	return nil
}

// notFoundError implements the errdefs.ErrNotFound interface for test mocks.
type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }
func (notFoundError) NotFound()     {}
