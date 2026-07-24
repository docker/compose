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
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/coordinator"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func newPushTestService(t *testing.T, apiClient *mocks.MockAPIClient, version string) *composeService {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: version}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return(version).AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	return tested.(*composeService)
}

func dialerFor(addr string) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
}

func newUpTestProject() *types.Project {
	return &types.Project{
		Name: "test",
		Services: types.Services{
			"web": {Name: "web", Image: "nginx"},
		},
	}
}

// TestPushProjectConfigGlue covers the pkg/compose glue: it negotiates the
// engine API version and delegates to the coordinator client, which pushes to
// the version-negotiated endpoint reached over the engine dialer.
func TestPushProjectConfigGlue(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	apiClient.EXPECT().Dialer().Return(dialerFor(srv.Listener.Addr().String())).AnyTimes()
	s := newPushTestService(t, apiClient, coordinator.MinAPIVersion)

	err := s.pushProjectConfig(t.Context(), newUpTestProject(), true)
	assert.NilError(t, err)
	assert.Equal(t, gotPath, "/v"+coordinator.MinAPIVersion+"/compose/project")
}

// TestPushProjectConfigGlueVersionError covers the glue's error branch when
// API-version negotiation fails; the coordinator dialer is never reached.
func TestPushProjectConfigGlueVersionError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{}, errors.New("engine unreachable")).AnyTimes()
	// The dialer must never be reached when negotiation fails.
	apiClient.EXPECT().Dialer().Times(0)

	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	s := tested.(*composeService)

	err = s.pushProjectConfig(t.Context(), newUpTestProject(), true)
	assert.ErrorContains(t, err, "negotiating API version")
	assert.ErrorContains(t, err, "engine unreachable")
}

func TestHasActiveProfiles(t *testing.T) {
	tests := []struct {
		name     string
		profiles []string
		want     bool
	}{
		{name: "nil", profiles: nil, want: false},
		{name: "empty slice", profiles: []string{}, want: false},
		{name: "single blank (default, no profile)", profiles: []string{""}, want: false},
		{name: "named profile", profiles: []string{"debug"}, want: true},
		{name: "blank plus named", profiles: []string{"", "debug"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, hasActiveProfiles(tt.profiles), tt.want)
		})
	}
}

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
