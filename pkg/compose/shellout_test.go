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
	"os"
	"path/filepath"
	"strings"
	"testing"

	clicontext "github.com/docker/cli/cli/context"
	"github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/flags"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

// TestPropagateDockerEndpoint locks which endpoint a child CLI plugin gets
// (#14433): a named context by name only, the "default" one (also the one in
// use with -H or DOCKER_HOST) by its host and TLS material.
func TestPropagateDockerEndpoint(t *testing.T) {
	tlsData := &clicontext.TLSData{CA: []byte("ca"), Cert: []byte("cert"), Key: []byte("key")}
	tests := []struct {
		name     string
		context  string
		endpoint docker.Endpoint
		wantHost string
		wantTLS  bool
	}{
		{
			name:     "named context",
			context:  "desktop-linux",
			endpoint: docker.Endpoint{EndpointMeta: docker.EndpointMeta{Host: "unix:///var/run/desktop.sock"}},
		},
		{
			name:     "named context with TLS",
			context:  "remote-tls",
			endpoint: docker.Endpoint{EndpointMeta: docker.EndpointMeta{Host: "tcp://remote:2376"}, TLSData: tlsData},
		},
		{
			name:     "default context",
			context:  "default",
			endpoint: docker.Endpoint{EndpointMeta: docker.EndpointMeta{Host: "ssh://user@remote"}},
			wantHost: "ssh://user@remote",
		},
		{
			name:     "default context with TLS",
			context:  "default",
			endpoint: docker.Endpoint{EndpointMeta: docker.EndpointMeta{Host: "tcp://remote:2376"}, TLSData: tlsData},
			wantHost: "tcp://remote:2376",
			wantTLS:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := mocks.NewMockCli(gomock.NewController(t))
			cli.EXPECT().CurrentContext().Return(tt.context).AnyTimes()
			cli.EXPECT().DockerEndpoint().Return(tt.endpoint).AnyTimes()
			s := &composeService{dockerCli: cli}

			values, cleanup, err := s.propagateDockerEndpoint()
			assert.NilError(t, err)
			defer cleanup()
			env := map[string]string{}
			for _, v := range values {
				k, val, _ := strings.Cut(v, "=")
				env[k] = val
			}

			assert.Equal(t, env["DOCKER_CONTEXT"], tt.context)
			assert.Equal(t, env["DOCKER_HOST"], tt.wantHost)
			certs, hasCerts := env["DOCKER_CERT_PATH"]
			assert.Equal(t, hasCerts, tt.wantTLS)
			if tt.wantTLS {
				assert.Equal(t, env["DOCKER_TLS_VERIFY"], "1")
				_, err := os.Stat(filepath.Join(certs, flags.DefaultCaFile))
				assert.NilError(t, err)
			}
		})
	}
}
