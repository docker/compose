/*
   Copyright 2022 Docker Compose CLI authors

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
	"strings"
	"sync"
	"testing"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compose "github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
)

func TestComposeService_Logs_Demux(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}
	cli.EXPECT().Client().Return(api).AnyTimes()

	name := strings.ToLower(testProject)

	ctx := context.Background()
	api.EXPECT().ContainerList(ctx, moby.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(oneOffFilter(false), projectFilter(name)),
	}).Return(
		[]moby.Container{
			testContainer("service", "c", false),
		},
		nil,
	)

	api.EXPECT().
		ContainerInspect(anyCancellableContext(), "c").
		Return(moby.ContainerJSON{
			ContainerJSONBase: &moby.ContainerJSONBase{ID: "c"},
			Config:            &container.Config{Tty: false},
		}, nil)
	c1Reader, c1Writer := io.Pipe()
	t.Cleanup(func() {
		_ = c1Reader.Close()
		_ = c1Writer.Close()
	})
	c1Stdout := stdcopy.NewStdWriter(c1Writer, stdcopy.Stdout)
	c1Stderr := stdcopy.NewStdWriter(c1Writer, stdcopy.Stderr)
	go func() {
		_, err := c1Stdout.Write([]byte("hello stdout\n"))
		assert.NoError(t, err, "Writing to fake stdout")
		_, err = c1Stderr.Write([]byte("hello stderr\n"))
		assert.NoError(t, err, "Writing to fake stderr")
		_ = c1Writer.Close()
	}()
	api.EXPECT().ContainerLogs(anyCancellableContext(), "c", gomock.Any()).
		Return(c1Reader, nil)

	opts := compose.LogOptions{
		Project: &types.Project{
			Services: types.Services{
				{Name: "service"},
			},
		},
	}

	consumer := &testLogConsumer{}
	err := tested.Logs(ctx, name, consumer, opts)
	require.NoError(t, err)

	require.Equal(
		t,
		[]string{"hello stdout", "hello stderr"},
		consumer.LogsForContainer("service", "c"),
	)
}

// TestComposeService_Logs_ServiceFiltering ensures that we do not include
// logs from out-of-scope services based on the Compose file vs actual state.
//
// NOTE(milas): This test exists because each method is currently duplicating
// a lot of the project/service filtering logic. We should consider moving it
// to an earlier point in the loading process, at which point this test could
// safely be removed.
func TestComposeService_Logs_ServiceFiltering(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}
	cli.EXPECT().Client().Return(api).AnyTimes()

	name := strings.ToLower(testProject)

	ctx := context.Background()
	api.EXPECT().ContainerList(ctx, moby.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(oneOffFilter(false), projectFilter(name)),
	}).Return(
		[]moby.Container{
			testContainer("serviceA", "c1", false),
			testContainer("serviceA", "c2", false),
			// serviceB will be filtered out by the project definition to
			// ensure we ignore "orphan" containers
			testContainer("serviceB", "c3", false),
			testContainer("serviceC", "c4", false),
		},
		nil,
	)

	for _, id := range []string{"c1", "c2", "c4"} {
		id := id
		api.EXPECT().
			ContainerInspect(anyCancellableContext(), id).
			Return(
				moby.ContainerJSON{
					ContainerJSONBase: &moby.ContainerJSONBase{ID: id},
					Config:            &container.Config{Tty: true},
				},
				nil,
			)
		api.EXPECT().ContainerLogs(anyCancellableContext(), id, gomock.Any()).
			Return(io.NopCloser(strings.NewReader("hello "+id+"\n")), nil).
			Times(1)
	}

	// this simulates passing `--filename` with a Compose file that does NOT
	// reference `serviceB` even though it has running services for this proj
	proj := &types.Project{
		Services: types.Services{
			{Name: "serviceA"},
			{Name: "serviceC"},
		},
	}
	consumer := &testLogConsumer{}
	opts := compose.LogOptions{
		Project: proj,
	}
	err := tested.Logs(ctx, name, consumer, opts)
	require.NoError(t, err)

	require.Equal(t, []string{"hello c1"}, consumer.LogsForContainer("serviceA", "c1"))
	require.Equal(t, []string{"hello c2"}, consumer.LogsForContainer("serviceA", "c2"))
	require.Empty(t, consumer.LogsForContainer("serviceB", "c3"))
	require.Equal(t, []string{"hello c4"}, consumer.LogsForContainer("serviceC", "c4"))
}

type testLogConsumer struct {
	mu sync.Mutex
	// logs is keyed by service, then container; values are log lines
	logs map[string]map[string][]string
}

func (l *testLogConsumer) Log(containerName, service, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logs == nil {
		l.logs = make(map[string]map[string][]string)
	}
	if l.logs[service] == nil {
		l.logs[service] = make(map[string][]string)
	}
	l.logs[service][containerName] = append(l.logs[service][containerName], message)
}

func (l *testLogConsumer) Status(containerName, msg string) {}

func (l *testLogConsumer) Register(containerName string) {}

func (l *testLogConsumer) LogsForContainer(svc string, containerName string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.logs[svc][containerName]
}
