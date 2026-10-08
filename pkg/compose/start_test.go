//go:build !windows

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
	"sync"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// recordingEventProcessor captures progress events for sequence assertions.
// Shared by tests elsewhere in the package that exercise a single container
// operation in isolation (e.g. stopContainer's relay guard in down_test.go).
type recordingEventProcessor struct {
	mu     sync.Mutex
	events []api.Resource
}

func (r *recordingEventProcessor) Start(context.Context, string) {}

func (r *recordingEventProcessor) On(events ...api.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

func (r *recordingEventProcessor) Done(string, bool) {}

// summary renders recorded events as "ID: Text" strings.
func (r *recordingEventProcessor) summary() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.ID+": "+e.Text)
	}
	return out
}

func (r *recordingEventProcessor) contains(entry string) bool {
	for _, e := range r.summary() {
		if e == entry {
			return true
		}
	}
	return false
}

// newStartTestService builds a composeService on gomock with a recording
// event processor. Any API call without a matching expectation fails the test,
// so "no expectation" doubles as a "no daemon interaction" assertion.
func newStartTestService(t *testing.T) (*composeService, *mocks.MockAPIClient, *recordingEventProcessor) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()
	// the generic pre_start inheritance goes through getCreateConfigs, which
	// reads the CLI configuration and the daemon host
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("unix:///var/run/docker.sock").AnyTimes()

	rec := &recordingEventProcessor{}
	svc, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)
	return svc.(*composeService), apiClient, rec
}

// serviceContainer builds a running container summary the way the daemon
// reports a project's first replica: canonical name, service and
// container-number labels.
func serviceContainer(service string) container.Summary {
	name := "prj-" + service + "-1"
	return container.Summary{
		ID:    name + "-id",
		Names: []string{"/" + name},
		State: container.StateRunning,
		Labels: map[string]string{
			api.ServiceLabel:         service,
			api.ContainerNumberLabel: "1",
		},
	}
}

// TestGetDependencyCondition locks the --wait condition selection: a service
// that others depend on with service_completed_successfully is waited on with
// that condition (or --wait would hang on one-shot services), anything else
// with running_or_healthy.
func TestGetDependencyCondition(t *testing.T) {
	oneShot := types.ServiceConfig{Name: "migrate"}
	web := types.ServiceConfig{
		Name: "web", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
			"migrate": {Condition: types.ServiceConditionCompletedSuccessfully},
		}},
	}
	project := &types.Project{
		Name:     "prj",
		Services: types.Services{"web": web, "migrate": oneShot},
	}

	assert.Equal(t, getDependencyCondition(oneShot, project), types.ServiceConditionCompletedSuccessfully)
	assert.Equal(t, getDependencyCondition(web, project), ServiceConditionRunningOrHealthy)
}
