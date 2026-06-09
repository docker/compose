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
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

type testRemoteLoader struct {
	localPath string
}

func (l testRemoteLoader) Accept(path string) bool {
	return strings.HasPrefix(path, "test://")
}

func (l testRemoteLoader) Load(context.Context, string) (string, error) {
	return l.localPath, nil
}

func (l testRemoteLoader) Dir(string) string {
	return filepath.Dir(l.localPath)
}

var _ loader.ResourceLoader = testRemoteLoader{}

func TestApplyScaleOpt(t *testing.T) {
	p := types.Project{
		Services: types.Services{
			"foo": {
				Name: "foo",
			},
			"bar": {
				Name: "bar",
				Deploy: &types.DeployConfig{
					Mode: "test",
				},
			},
		},
	}
	err := applyScaleOpts(&p, []string{"foo=2", "bar=3"})
	assert.NilError(t, err)
	foo, err := p.GetService("foo")
	assert.NilError(t, err)
	assert.Equal(t, *foo.Scale, 2)

	bar, err := p.GetService("bar")
	assert.NilError(t, err)
	assert.Equal(t, *bar.Scale, 3)
	assert.Equal(t, *bar.Deploy.Replicas, 3)
}

func TestUpOptions_OnExit(t *testing.T) {
	tests := []struct {
		name string
		args upOptions
		want api.Cascade
	}{
		{
			name: "no cascade",
			args: upOptions{},
			want: api.CascadeIgnore,
		},
		{
			name: "cascade stop",
			args: upOptions{cascadeStop: true},
			want: api.CascadeStop,
		},
		{
			name: "cascade fail",
			args: upOptions{cascadeFail: true},
			want: api.CascadeFail,
		},
		{
			name: "both set - stop takes precedence",
			args: upOptions{
				cascadeStop: true,
				cascadeFail: true,
			},
			want: api.CascadeStop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.args.OnExit()
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestRunUpAllowsTemplatedPortFieldsInRemoteStackPrompt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	assert.NilError(t, os.WriteFile(composePath, []byte(`
name: remote-defaults
services:
  web:
    image: nginx
    ports:
      - host_ip: "${LXKNS_ADDRESS:-127.0.0.1}"
        published: "${LXKNS_PORT:-5010}"
        target: 80
        protocol: tcp
`), 0o600))

	in := io.NopCloser(bytes.NewBufferString("n\n"))
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cli := mocks.NewMockCli(ctrl)
	cli.EXPECT().In().Return(streams.NewIn(in)).AnyTimes()
	cli.EXPECT().Out().Return(streams.NewOut(out)).AnyTimes()
	cli.EXPECT().Err().Return(streams.NewOut(errOut)).AnyTimes()

	projectOptions := &ProjectOptions{
		ConfigPaths:           []string{"test://remote/compose.yaml"},
		ProjectDir:            dir,
		remoteLoadersOverride: []loader.ResourceLoader{testRemoteLoader{localPath: composePath}},
	}
	project := &types.Project{
		Name:       "remote-defaults",
		WorkingDir: dir,
		Services: types.Services{
			"web": {
				Name:  "web",
				Image: "nginx",
			},
		},
	}

	err := runUp(
		t.Context(),
		cli,
		&BackendOptions{},
		createOptions{},
		upOptions{},
		buildOptions{ProjectOptions: projectOptions},
		project,
		nil,
	)

	assert.Error(t, err, "operation cancelled by user")
	output := out.String()
	assert.Assert(t, strings.Contains(output, `Your compose stack "test://remote/compose.yaml"`), output)
	assert.Assert(t, strings.Contains(output, "LXKNS_ADDRESS"), output)
	assert.Assert(t, strings.Contains(output, "LXKNS_PORT"), output)
	assert.Assert(t, !strings.Contains(fmt.Sprint(err), "invalid ip address"), fmt.Sprint(err))
}
