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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/docker"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

func TestResolvePullPlatforms(t *testing.T) {
	tests := []struct {
		name            string
		servicePlatform string
		defaultPlatform string
		wantLen         int
		wantErr         bool
	}{
		{name: "none", wantLen: 0},
		{name: "service platform", servicePlatform: "linux/amd64", wantLen: 1},
		{name: "default platform fallback", defaultPlatform: "linux/arm64", wantLen: 1},
		{name: "service overrides default", servicePlatform: "linux/amd64", defaultPlatform: "linux/arm64", wantLen: 1},
		{name: "invalid", servicePlatform: "not a platform!!", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePullPlatforms(tt.servicePlatform, tt.defaultPlatform)
			if tt.wantErr {
				assert.Assert(t, err != nil)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, len(got), tt.wantLen)
		})
	}
}

// TestServiceClientHeaders verifies that requests made through a per-service
// client carry headers identifying the Compose project and service on whose
// behalf the request is made, and that CLI-configured headers are preserved.
func TestServiceClientHeaders(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	var gotProject, gotService, gotConfigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/images/create") {
			gotProject = r.Header.Get(composeProjectHeader)
			gotService = r.Header.Get(composeServiceHeader)
			gotConfigHeader = r.Header.Get("X-Custom")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"Pulling"}` + "\n"))
	}))
	defer srv.Close()

	api, cli := prepareMocks(mockCtrl)
	// Use a "tcp" scheme so the moby client dials the httptest server over HTTP.
	host := "tcp://" + strings.TrimPrefix(srv.URL, "http://")
	cli.EXPECT().DockerEndpoint().Return(docker.Endpoint{
		EndpointMeta: docker.EndpointMeta{Host: host},
	}).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{
		HTTPHeaders: map[string]string{"X-Custom": "config-value"},
	}).AnyTimes()
	api.EXPECT().ClientVersion().Return("1.51").AnyTimes()
	api.EXPECT().Close().Return(nil).AnyTimes()

	s := composeService{dockerCli: cli}

	apiClient, err := s.serviceClient("myProject", "myService")
	assert.NilError(t, err)

	stream, err := apiClient.ImagePull(t.Context(), "alpine:latest", client.ImagePullOptions{})
	assert.NilError(t, err)
	_, _ = io.Copy(io.Discard, stream)
	_ = stream.Close()

	assert.Equal(t, gotProject, "myProject")
	assert.Equal(t, gotService, "myService")
	// Headers configured on the CLI (e.g. via config.json) are preserved.
	assert.Equal(t, gotConfigHeader, "config-value")

	// The same service resolves to the cached client; a different service does not.
	again, err := s.serviceClient("myProject", "myService")
	assert.NilError(t, err)
	assert.Equal(t, again, apiClient)
	other, err := s.serviceClient("myProject", "otherService")
	assert.NilError(t, err)
	assert.Assert(t, other != apiClient)

	assert.NilError(t, s.Close())
}
