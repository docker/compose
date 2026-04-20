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
	"testing"

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
