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
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPsTable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	out := filepath.Join(dir, "output.txt")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal("could not create output file")
	}
	defer func() { _ = f.Close() }()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	backend := mocks.NewMockService(ctrl)
	backend.EXPECT().
		Ps(gomock.Eq(ctx), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, projectName string, options api.PsOptions) ([]api.ContainerSummary, error) {
			return []api.ContainerSummary{
				{
					ID:    "abc123",
					Name:  "ABC",
					Image: "foo/bar",
					Publishers: api.PortPublishers{
						{
							TargetPort:    8080,
							PublishedPort: 8080,
							Protocol:      "tcp",
						},
						{
							TargetPort:    8443,
							PublishedPort: 8443,
							Protocol:      "tcp",
						},
					},
				},
			}, nil
		}).AnyTimes()

	opts := psOptions{ProjectOptions: &ProjectOptions{ProjectName: "test"}}
	stdout := streams.NewOut(f)
	cli := mocks.NewMockCli(ctrl)
	cli.EXPECT().Out().Return(stdout).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	err = runPs(ctx, cli, backend, nil, opts)
	require.NoError(t, err)

	_, err = f.Seek(0, 0)
	require.NoError(t, err)

	output, err := os.ReadFile(out)
	require.NoError(t, err)

	assert.Contains(t, string(output), "8080/tcp, 8443/tcp")
}
