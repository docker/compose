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
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func TestFilterServices(t *testing.T) {
	p := &types.Project{
		Services: types.Services{
			"foo": {
				Name:  "foo",
				Links: []string{"bar"},
			},
			"bar": {
				Name: "bar",
				DependsOn: map[string]types.ServiceDependency{
					"zot": {},
				},
			},
			"zot": {
				Name: "zot",
			},
			"qix": {
				Name: "qix",
			},
		},
	}
	p, err := p.WithSelectedServices([]string{"bar"})
	assert.NilError(t, err)

	assert.Equal(t, len(p.Services), 2)
	_, err = p.GetService("bar")
	assert.NilError(t, err)
	_, err = p.GetService("zot")
	assert.NilError(t, err)
}

func TestUpLoadsConfigBeforeDockerConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wd := t.TempDir()
	t.Chdir(wd)
	configDir := t.TempDir()
	t.Setenv("COMPOSE_FILE", configDir)

	cli := mocks.NewMockCli(ctrl)

	cmd := upCommand(&ProjectOptions{}, cli, &BackendOptions{})
	cmd.SetContext(t.Context())
	cmd.SetArgs([]string{"-d"})

	err := cmd.Execute()

	assert.ErrorContains(t, err, "is a directory")
}

func TestUpChecksDockerConnectionBeforeDefaultConfigDiscovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wd := t.TempDir()
	t.Chdir(wd)

	socketErr := errors.New("permission denied while trying to connect to the docker API at unix:///var/run/docker.sock")
	apiClient := mocks.NewMockAPIClient(ctrl)
	apiClient.EXPECT().
		Ping(gomock.Any(), client.PingOptions{}).
		Return(client.PingResult{}, socketErr)

	cli := mocks.NewMockCli(ctrl)
	cli.EXPECT().Client().Return(apiClient)

	cmd := upCommand(&ProjectOptions{}, cli, &BackendOptions{})
	cmd.SetContext(t.Context())
	cmd.SetArgs([]string{"-d"})

	err := cmd.Execute()

	assert.ErrorIs(t, err, socketErr)
}
