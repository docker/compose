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

package coordinator

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/store"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

var testStoreCfg = store.NewConfig(
	func() any {
		return &map[string]any{}
	},
)

func newContextStore(t *testing.T, meta any) store.Store {
	t.Helper()
	st := store.New(t.TempDir(), testStoreCfg)
	err := st.CreateOrUpdate(store.Metadata{
		Name:      "test",
		Metadata:  meta,
		Endpoints: make(map[string]any),
	})
	assert.NilError(t, err)
	return st
}

func TestPushEnabled(t *testing.T) {
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
			meta: dockerContext(map[string]any{MetadataKey: true}),
			want: true,
		},
		{
			name: "string true",
			meta: dockerContext(map[string]any{MetadataKey: "true"}),
			want: true,
		},
		{
			name: "string TRUE case-insensitive",
			meta: dockerContext(map[string]any{MetadataKey: "TRUE"}),
			want: true,
		},
		{
			name: "boolean false",
			meta: dockerContext(map[string]any{MetadataKey: false}),
			want: false,
		},
		{
			name: "string other",
			meta: dockerContext(map[string]any{MetadataKey: "yes"}),
			want: false,
		},
		{
			name: "key absent",
			meta: dockerContext(map[string]any{"other": true}),
			want: false,
		},
		{
			name: "raw map form",
			meta: map[string]any{MetadataKey: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			cli := mocks.NewMockCli(mockCtrl)
			cli.EXPECT().ContextStore().Return(newContextStore(t, tt.meta)).AnyTimes()
			cli.EXPECT().CurrentContext().Return("test").AnyTimes()

			assert.Equal(t, PushEnabled(cli), tt.want)
		})
	}
}

func TestPushEnabledMetadataError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	// An empty store returns an error for an unknown context; that must be
	// treated as "not enabled" rather than blocking up.
	cli.EXPECT().ContextStore().Return(store.New(t.TempDir(), testStoreCfg)).AnyTimes()
	cli.EXPECT().CurrentContext().Return("missing").AnyTimes()

	assert.Equal(t, PushEnabled(cli), false)
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
		gotMethod   string
		gotPath     string
		gotBody     []byte
		gotComplete string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		gotComplete = r.Header.Get(CompleteHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(dialerFor(srv.Listener.Addr().String()))
	err := c.PushProjectConfig(t.Context(), MinAPIVersion, newTestProject(), true)
	assert.NilError(t, err)
	assert.Equal(t, gotMethod, http.MethodPost)
	assert.Equal(t, gotPath, "/v"+MinAPIVersion+"/compose/project")
	assert.Assert(t, len(gotBody) > 0)
	assert.Equal(t, gotComplete, "true")
}

func TestPushProjectConfigSubsetHeader(t *testing.T) {
	// A subset push (empty-selection == false) must advertise itself so the
	// coordinator merges rather than prunes.
	var gotComplete string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotComplete = r.Header.Get(CompleteHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(dialerFor(srv.Listener.Addr().String()))
	err := c.PushProjectConfig(t.Context(), MinAPIVersion, newTestProject(), false)
	assert.NilError(t, err)
	assert.Equal(t, gotComplete, "false")
}

func TestPushProjectConfigServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "placement failed", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(dialerFor(srv.Listener.Addr().String()))
	err := c.PushProjectConfig(t.Context(), MinAPIVersion, newTestProject(), true)
	assert.ErrorContains(t, err, "500")
	assert.ErrorContains(t, err, "placement failed")
}

func TestPushProjectConfigDialerError(t *testing.T) {
	// A dialer that never connects surfaces as a request error, which callers
	// treat as non-fatal.
	dialer := func(context.Context) (net.Conn, error) {
		return nil, errors.New("boom: no engine socket")
	}
	c := NewClient(dialer)
	err := c.PushProjectConfig(t.Context(), MinAPIVersion, newTestProject(), true)
	assert.ErrorContains(t, err, "boom: no engine socket")
}

func TestPushProjectConfigErrorEmptyBody(t *testing.T) {
	// A non-2xx status with no body still yields a useful error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(dialerFor(srv.Listener.Addr().String()))
	err := c.PushProjectConfig(t.Context(), MinAPIVersion, newTestProject(), true)
	assert.ErrorContains(t, err, "coordinator returned status 503")
}

func TestPushProjectConfigVersionTooLow(t *testing.T) {
	// The dialer must never be reached: the version gate rejects first.
	dialer := func(context.Context) (net.Conn, error) {
		t.Fatal("dialer should not be called when the API version is too low")
		return nil, nil
	}
	c := NewClient(dialer)
	err := c.PushProjectConfig(t.Context(), "1.44", newTestProject(), true)
	assert.ErrorContains(t, err, "does not support the project-config push")
}

func TestPushProjectConfigTimeout(t *testing.T) {
	// A coordinator that accepts the connection but never responds must not
	// hang the push: the client timeout bounds the request.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	c := NewClient(dialerFor(srv.Listener.Addr().String()))
	c.timeout = 50 * time.Millisecond

	err := c.PushProjectConfig(t.Context(), MinAPIVersion, newTestProject(), true)
	assert.ErrorContains(t, err, "context deadline exceeded")
}
