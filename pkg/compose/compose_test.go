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
	"fmt"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// TestNewLimitedErrgroup_NonPositiveIsUnlimited guards against the bug this
// helper exists to fix: errgroup.SetLimit(0) means "allow zero goroutines",
// not "unlimited". A composeService{} built without going through
// NewComposeService has maxConcurrency's Go zero-value (0), so an
// unconditional SetLimit(maxConcurrency) at any call site would silently
// hang forever instead of running.
func TestNewLimitedErrgroup_NonPositiveIsUnlimited(t *testing.T) {
	for _, maxConcurrency := range []int{0, -1} {
		t.Run(fmt.Sprintf("maxConcurrency=%d", maxConcurrency), func(t *testing.T) {
			eg, _ := newLimitedErrgroup(t.Context(), maxConcurrency)

			done := make(chan struct{})
			go func() {
				for range 5 {
					eg.Go(func() error { return nil })
				}
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("eg.Go blocked: maxConcurrency <= 0 must mean unlimited, not SetLimit(0) (zero goroutines allowed)")
			}
			assert.NilError(t, eg.Wait())
		})
	}
}

// TestProjectFromName_DependsOnRequired guards against a project rebuilt from
// container labels (`-p <name>` without `-f`) turning an optional dependency
// into a required one: `start` then failed on a dependency the compose file
// marked `required: false`.
func TestProjectFromName_DependsOnRequired(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		required bool
	}{
		{name: "optional dependency", label: "init:service_completed_successfully:false:false", required: false},
		{name: "required dependency", label: "init:service_completed_successfully:false:true", required: true},
		{name: "label without the required field", label: "init:service_completed_successfully:false", required: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containers := Containers{
				{Labels: map[string]string{api.ServiceLabel: "web", api.DependenciesLabel: tt.label}},
				{Labels: map[string]string{api.ServiceLabel: "init"}},
			}
			project, err := (&composeService{}).projectFromName(containers, "prj")
			assert.NilError(t, err)
			web, err := project.GetService("web")
			assert.NilError(t, err)
			assert.Equal(t, web.DependsOn["init"].Required, tt.required)
		})
	}
}

// TestDependsOnLabel_RoundTripsRequired locks both halves of the label: what
// getCreateConfigs writes on a container must read back through
// projectFromName with the same `required` value, for optional and required
// dependencies alike.
func TestDependsOnLabel_RoundTripsRequired(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(fmt.Sprintf("required=%t", required), func(t *testing.T) {
			svc, apiClient := newTestService(t)
			apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
				Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
			apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()

			web := types.ServiceConfig{
				Name:          "web",
				ContainerSpec: types.ContainerSpec{Image: "alpine"},
				WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
					"init": {Condition: types.ServiceConditionCompletedSuccessfully, Required: required},
				}},
			}
			project := &types.Project{Name: "prj", Services: types.Services{"web": web}}

			cfgs, err := svc.getCreateConfigs(t.Context(), project, web, 1, nil, createOptions{
				Labels: types.Labels{api.ServiceLabel: "web"},
			})
			assert.NilError(t, err)

			containers := Containers{
				{Labels: cfgs.Container.Labels},
				{Labels: map[string]string{api.ServiceLabel: "init"}},
			}
			rebuilt, err := svc.projectFromName(containers, "prj")
			assert.NilError(t, err)
			got, err := rebuilt.GetService("web")
			assert.NilError(t, err)
			assert.Equal(t, got.DependsOn["init"].Required, required)
		})
	}
}
