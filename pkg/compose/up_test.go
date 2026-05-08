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
	"bytes"
	"strings"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestShouldFollowStartEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    api.ContainerEvent
		attached []string
		attachTo []string
		want     bool
	}{
		{
			name: "ignores non-start events",
			event: api.ContainerEvent{
				Type:    api.ContainerEventExited,
				Service: "validator",
			},
			attachTo: []string{"validator"},
			want:     false,
		},
		{
			name: "ignores services outside explicit attach selection",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "event-bus-validator",
				ID:      "event-bus-validator-1",
			},
			attachTo: []string{"validator"},
			want:     false,
		},
		{
			name: "ignores containers already attached unless restarting",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "validator",
				ID:      "validator-1",
			},
			attached: []string{"validator-1"},
			attachTo: []string{"validator"},
			want:     false,
		},
		{
			name: "follows restarts for attached service",
			event: api.ContainerEvent{
				Type:       api.ContainerEventStarted,
				Service:    "validator",
				ID:         "validator-1",
				Restarting: true,
			},
			attached: []string{"validator-1"},
			attachTo: []string{"validator"},
			want:     true,
		},
		{
			name: "follows selected service when not already attached",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "validator",
				ID:      "validator-2",
			},
			attachTo: []string{"validator"},
			want:     true,
		},
		{
			name: "follows service when no explicit attach filter exists",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "validator",
				ID:      "validator-2",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFollowStartEvent(tt.event, tt.attached, tt.attachTo)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestWarnExternalNetworkAliases(t *testing.T) {
	tests := []struct {
		name        string
		project     *composetypes.Project
		expected    []string
		notExpected []string
	}{
		{
			name: "internal-only network emits no warning",
			project: &composetypes.Project{
				Networks: composetypes.Networks{"internal": {}},
				Services: composetypes.Services{
					"web": {
						Name:     "web",
						Networks: map[string]*composetypes.ServiceNetworkConfig{"internal": nil},
					},
				},
			},
			notExpected: []string{"not registered as aliases"},
		},
		{
			name: "external network emits one warning listing services in sorted order",
			project: &composetypes.Project{
				Networks: composetypes.Networks{"shared": {External: true}},
				Services: composetypes.Services{
					"web": {
						Name:     "web",
						Networks: map[string]*composetypes.ServiceNetworkConfig{"shared": nil},
					},
					"db": {
						Name:     "db",
						Networks: map[string]*composetypes.ServiceNetworkConfig{"shared": nil},
					},
				},
			},
			expected: []string{
				`service names [db, web] not registered as aliases on external network`,
				`networks.shared.aliases`,
			},
		},
		{
			name: "service with explicit self-alias is excluded from the warning",
			project: &composetypes.Project{
				Networks: composetypes.Networks{"shared": {External: true}},
				Services: composetypes.Services{
					"web": {
						Name: "web",
						Networks: map[string]*composetypes.ServiceNetworkConfig{
							"shared": {Aliases: []string{"web"}},
						},
					},
					"db": {
						Name:     "db",
						Networks: map[string]*composetypes.ServiceNetworkConfig{"shared": nil},
					},
				},
			},
			expected: []string{
				`service names [db] not registered as aliases on external network`,
				`networks.shared.aliases`,
			},
			notExpected: []string{"web"},
		},
		{
			name: "all services explicitly self-aliased emits no warning",
			project: &composetypes.Project{
				Networks: composetypes.Networks{"shared": {External: true}},
				Services: composetypes.Services{
					"web": {
						Name: "web",
						Networks: map[string]*composetypes.ServiceNetworkConfig{
							"shared": {Aliases: []string{"web"}},
						},
					},
				},
			},
			notExpected: []string{"not registered as aliases"},
		},
		{
			name: "multiple external networks each emit their own warning",
			project: &composetypes.Project{
				Networks: composetypes.Networks{
					"sharedA": {External: true},
					"sharedB": {External: true},
				},
				Services: composetypes.Services{
					"web": {
						Name: "web",
						Networks: map[string]*composetypes.ServiceNetworkConfig{
							"sharedA": nil,
							"sharedB": nil,
						},
					},
				},
			},
			expected: []string{
				`networks.sharedA.aliases`,
				`networks.sharedB.aliases`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			origOut := logrus.StandardLogger().Out
			origLevel := logrus.GetLevel()
			origFormatter := logrus.StandardLogger().Formatter
			logrus.SetOutput(&buf)
			logrus.SetLevel(logrus.WarnLevel)
			logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
			t.Cleanup(func() {
				logrus.SetOutput(origOut)
				logrus.SetLevel(origLevel)
				logrus.SetFormatter(origFormatter)
			})

			warnExternalNetworkAliases(tt.project)

			out := buf.String()
			for _, s := range tt.expected {
				assert.Assert(t, strings.Contains(out, s), "expected %q in output:\n%s", s, out)
			}
			for _, s := range tt.notExpected {
				assert.Assert(t, !strings.Contains(out, s), "did not expect %q in output:\n%s", s, out)
			}
		})
	}
}
