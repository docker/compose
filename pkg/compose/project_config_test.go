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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/store"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

var testProjectConfigStoreCfg = store.NewConfig(
	func() any {
		return &map[string]any{}
	},
)

func newProjectConfigStore(t *testing.T, meta any) store.Store {
	t.Helper()
	st := store.New(t.TempDir(), testProjectConfigStoreCfg)
	err := st.CreateOrUpdate(store.Metadata{
		Name:      "test",
		Metadata:  meta,
		Endpoints: make(map[string]any),
	})
	assert.NilError(t, err)
	return st
}

func TestProjectConfigPushEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("Requires filesystem access")
	}

	dockerContext := func(fields map[string]any) command.DockerContext {
		return command.DockerContext{Description: "test", AdditionalFields: fields}
	}

	tests := []struct {
		name string
		meta any
		want bool
	}{
		{
			name: "boolean true",
			meta: dockerContext(map[string]any{projectConfigMetadataKey: true}),
			want: true,
		},
		{
			name: "string true",
			meta: dockerContext(map[string]any{projectConfigMetadataKey: "true"}),
			want: true,
		},
		{
			name: "string TRUE case-insensitive",
			meta: dockerContext(map[string]any{projectConfigMetadataKey: "TRUE"}),
			want: true,
		},
		{
			name: "boolean false",
			meta: dockerContext(map[string]any{projectConfigMetadataKey: false}),
			want: false,
		},
		{
			name: "string other",
			meta: dockerContext(map[string]any{projectConfigMetadataKey: "yes"}),
			want: false,
		},
		{
			name: "key absent",
			meta: dockerContext(map[string]any{"other": true}),
			want: false,
		},
		{
			name: "raw map form",
			meta: map[string]any{projectConfigMetadataKey: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			cli := mocks.NewMockCli(mockCtrl)
			cli.EXPECT().ContextStore().Return(newProjectConfigStore(t, tt.meta)).AnyTimes()
			cli.EXPECT().CurrentContext().Return("test").AnyTimes()

			s := &composeService{dockerCli: cli}
			assert.Equal(t, s.projectConfigPushEnabled(), tt.want)
		})
	}
}

func TestProjectConfigPushEnabledMetadataError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	// An empty store returns an error for an unknown context; that must be
	// treated as "not enabled" rather than blocking up.
	cli.EXPECT().ContextStore().Return(store.New(t.TempDir(), testProjectConfigStoreCfg)).AnyTimes()
	cli.EXPECT().CurrentContext().Return("missing").AnyTimes()

	s := &composeService{dockerCli: cli}
	assert.Equal(t, s.projectConfigPushEnabled(), false)
}

func newPushTestService(t *testing.T, version string) (*composeService, *mocks.MockAPIClient) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: version}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return(version).AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	return tested.(*composeService), apiClient
}

func newTestProject() *types.Project {
	return &types.Project{
		Name: "test",
		Services: types.Services{
			"web": {Name: "web", Image: "nginx"},
		},
	}
}

func dialerFor(addr string) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
}

func TestPushProjectConfig(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, apiClient := newPushTestService(t, apiVersionComposeProjectConfig)
	apiClient.EXPECT().Dialer().Return(dialerFor(srv.Listener.Addr().String())).AnyTimes()

	err := s.pushProjectConfig(t.Context(), newTestProject())
	assert.NilError(t, err)
	assert.Equal(t, gotMethod, http.MethodPost)
	assert.Equal(t, gotPath, "/v"+apiVersionComposeProjectConfig+"/compose/project")
	assert.Assert(t, len(gotBody) > 0)
}

func TestPushProjectConfigServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "placement failed", http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, apiClient := newPushTestService(t, apiVersionComposeProjectConfig)
	apiClient.EXPECT().Dialer().Return(dialerFor(srv.Listener.Addr().String())).AnyTimes()

	err := s.pushProjectConfig(t.Context(), newTestProject())
	assert.ErrorContains(t, err, "500")
	assert.ErrorContains(t, err, "placement failed")
}

func TestPushProjectConfigVersionTooLow(t *testing.T) {
	s, apiClient := newPushTestService(t, "1.44")
	// Dialer must never be reached: the version gate rejects first.
	apiClient.EXPECT().Dialer().Times(0)

	err := s.pushProjectConfig(t.Context(), newTestProject())
	assert.ErrorContains(t, err, "does not support the project-config push")
}
