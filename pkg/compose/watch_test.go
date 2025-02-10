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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/internal/sync"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
	"github.com/docker/compose/v2/pkg/watch"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

type testWatcher struct {
	events chan watch.FileEvent
	errors chan error
}

func (t testWatcher) Start() error {
	return nil
}

func (t testWatcher) Close() error {
	return nil
}

func (t testWatcher) Events() chan watch.FileEvent {
	return t.events
}

func (t testWatcher) Errors() chan error {
	return t.errors
}

type stdLogger struct{}

func (s stdLogger) Log(containerName, message string) {
	fmt.Printf("%s: %s\n", containerName, message)
}

func (s stdLogger) Err(containerName, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", containerName, message)
}

func (s stdLogger) Status(container, msg string) {
	fmt.Printf("%s: %s\n", container, msg)
}

func (s stdLogger) Register(container string) {
}

func TestWatch_Sync(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return([]moby.Container{
		testContainer("test", "123", false),
	}, nil).AnyTimes()
	// we expect the image to be pruned
	apiClient.EXPECT().ImageList(gomock.Any(), image.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("dangling", "true"),
			filters.Arg("label", api.ProjectLabel+"=myProjectName"),
		),
	}).Return([]image.Summary{
		{ID: "123"},
		{ID: "456"},
	}, nil).Times(1)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "123", image.RemoveOptions{}).Times(1)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "456", image.RemoveOptions{}).Times(1)
	//
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	proj := types.Project{
		Name: "myProjectName",
		Services: types.Services{
			"test": {
				Name: "test",
			},
		},
	}

	watcher := testWatcher{
		events: make(chan watch.FileEvent),
		errors: make(chan error),
	}

	syncer := newFakeSyncer()
	clock := clockwork.NewFakeClock()
	go func() {
		service := composeService{
			dockerCli: cli,
			clock:     clock,
		}
		rules, err := getWatchRules(&types.DevelopConfig{
			Watch: []types.Trigger{
				{
					Path:   "/sync",
					Action: "sync",
					Target: "/work",
					Ignore: []string{"ignore"},
				},
				{
					Path:   "/rebuild",
					Action: "rebuild",
				},
			},
		}, types.ServiceConfig{Name: "test"})
		assert.NilError(t, err)

		err = service.watchEvents(ctx, &proj, api.WatchOptions{
			Build: &api.BuildOptions{},
			LogTo: stdLogger{},
			Prune: true,
		}, watcher, syncer, rules)
		assert.NilError(t, err)
	}()

	watcher.Events() <- watch.NewFileEvent("/sync/changed")
	watcher.Events() <- watch.NewFileEvent("/sync/changed/sub")
	err := clock.BlockUntilContext(ctx, 3)
	assert.NilError(t, err)
	clock.Advance(watch.QuietPeriod)
	select {
	case actual := <-syncer.synced:
		require.ElementsMatch(t, []*sync.PathMapping{
			{HostPath: "/sync/changed", ContainerPath: "/work/changed"},
			{HostPath: "/sync/changed/sub", ContainerPath: "/work/changed/sub"},
		}, actual)
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout")
	}

	watcher.Events() <- watch.NewFileEvent("/rebuild")
	watcher.Events() <- watch.NewFileEvent("/sync/changed")
	err = clock.BlockUntilContext(ctx, 4)
	assert.NilError(t, err)
	clock.Advance(watch.QuietPeriod)
	select {
	case batch := <-syncer.synced:
		t.Fatalf("received unexpected events: %v", batch)
	case <-time.After(100 * time.Millisecond):
		// expected
	}
	// TODO: there's not a great way to assert that the rebuild attempt happened
}

type fakeSyncer struct {
	synced chan []*sync.PathMapping
}

func newFakeSyncer() *fakeSyncer {
	return &fakeSyncer{
		synced: make(chan []*sync.PathMapping),
	}
}

func (f *fakeSyncer) Sync(ctx context.Context, service string, paths []*sync.PathMapping) error {
	f.synced <- paths
	return nil
}
