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
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"github.com/jonboulle/clockwork"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/sync"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
	"github.com/docker/compose/v5/pkg/watch"
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

func (s stdLogger) Status(containerName, msg string) {
	fmt.Printf("%s: %s\n", containerName, msg)
}

func TestWatch_Sync(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{
		Items: []container.Summary{
			testContainer("test", "123", false),
		},
	}, nil).AnyTimes()
	// we expect the image to be pruned
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: make(client.Filters).
			Add("dangling", "true").
			Add("label", api.ProjectLabel+"=myProjectName"),
	}).Return(client.ImageListResult{
		Items: []image.Summary{
			{ID: "123"},
			{ID: "456"},
		},
	}, nil).Times(1)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "123", client.ImageRemoveOptions{}).Times(1)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "456", client.ImageRemoveOptions{}).Times(1)
	//
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	ctx, cancelFunc := context.WithCancel(t.Context())
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
		expected := []*sync.PathMapping{
			{HostPath: "/sync/changed", ContainerPath: "/work/changed"},
			{HostPath: "/sync/changed/sub", ContainerPath: "/work/changed/sub"},
		}
		slices.SortFunc(actual, func(a, b *sync.PathMapping) int {
			return cmp.Compare(a.HostPath, b.HostPath)
		})
		assert.DeepEqual(t, expected, actual)
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

// #13725: initialSyncFiles used to skip files whose mtime predated the image
// creation time, which silently dropped all pre-existing host files.
func TestInitialSyncFilesDirectory(t *testing.T) {
	hostDir := t.TempDir()
	hostFile := filepath.Join(hostDir, "test.txt")
	assert.NilError(t, os.WriteFile(hostFile, []byte("hello"), 0o600))
	// back-date the file to simulate a file that predates the image
	oldTime := time.Now().Add(-time.Hour)
	assert.NilError(t, os.Chtimes(hostFile, oldTime, oldTime))

	paths, err := (&composeService{}).initialSyncFiles(types.ServiceConfig{Name: "svc"}, types.Trigger{
		Path:   hostDir,
		Target: "/app/src",
	}, watch.EmptyMatcher{})
	assert.NilError(t, err)
	assert.DeepEqual(t, paths, []*sync.PathMapping{{
		HostPath:      hostFile,
		ContainerPath: "/app/src/test.txt",
	}})
}

// #13725: single-file trigger path was also gated on the image-creation-time
// check, preventing pre-existing files from being synced.
func TestInitialSyncFilesRegularFile(t *testing.T) {
	hostDir := t.TempDir()
	hostFile := filepath.Join(hostDir, "test.txt")
	assert.NilError(t, os.WriteFile(hostFile, []byte("hello"), 0o600))
	oldTime := time.Now().Add(-time.Hour)
	assert.NilError(t, os.Chtimes(hostFile, oldTime, oldTime))

	syncer := &fakeSyncer{synced: make(chan []*sync.PathMapping, 1)}
	err := (&composeService{}).initialSync(t.Context(), types.ServiceConfig{
		Name:  "svc",
		Build: &types.BuildConfig{Context: hostDir},
	}, types.Trigger{
		Path:   hostFile,
		Target: "/app/test.txt",
	}, syncer)
	assert.NilError(t, err)
	assert.DeepEqual(t, <-syncer.synced, []*sync.PathMapping{{
		HostPath:      hostFile,
		ContainerPath: "/app/test.txt",
	}})
}

// initialSync's doc comment promises the Dockerfile and compose files are
// never copied into the container, but a refactor (ed10804e0) dropped the
// matcher enforcing it without replacement — neither the .dockerignore-derived
// matcher nor EphemeralPathMatcher cover this.
func TestInitialSync_ExcludesDockerfileAndComposeFiles(t *testing.T) {
	hostDir := t.TempDir()
	for _, name := range []string{"Dockerfile", "compose.yaml", "docker-compose.yml", "compose.override.yml", "app.go"} {
		assert.NilError(t, os.WriteFile(filepath.Join(hostDir, name), []byte("content"), 0o600))
	}

	syncer := &fakeSyncer{synced: make(chan []*sync.PathMapping, 1)}
	err := (&composeService{}).initialSync(t.Context(), types.ServiceConfig{
		Name:  "svc",
		Build: &types.BuildConfig{Context: hostDir},
	}, types.Trigger{
		Path:   hostDir,
		Target: "/app",
	}, syncer)
	assert.NilError(t, err)

	paths := <-syncer.synced
	assert.DeepEqual(t, paths, []*sync.PathMapping{{
		HostPath:      filepath.Join(hostDir, "app.go"),
		ContainerPath: "/app/app.go",
	}})
}

// TestInitialSync_ExcludesCustomNamedDockerfile verifies that a service using
// a non-default Dockerfile name (build.dockerfile) still has it excluded from
// the initial sync, not just the literal "Dockerfile".
func TestInitialSync_ExcludesCustomNamedDockerfile(t *testing.T) {
	hostDir := t.TempDir()
	for _, name := range []string{"Dockerfile.prod", "app.go"} {
		assert.NilError(t, os.WriteFile(filepath.Join(hostDir, name), []byte("content"), 0o600))
	}

	syncer := &fakeSyncer{synced: make(chan []*sync.PathMapping, 1)}
	err := (&composeService{}).initialSync(t.Context(), types.ServiceConfig{
		Name:  "svc",
		Build: &types.BuildConfig{Context: hostDir, Dockerfile: "Dockerfile.prod"},
	}, types.Trigger{
		Path:   hostDir,
		Target: "/app",
	}, syncer)
	assert.NilError(t, err)

	paths := <-syncer.synced
	assert.DeepEqual(t, paths, []*sync.PathMapping{{
		HostPath:      filepath.Join(hostDir, "app.go"),
		ContainerPath: "/app/app.go",
	}})
}

// TestInitialSync_ExcludesNestedCustomNamedDockerfile verifies that a
// build.dockerfile living in a subdirectory of the build context (e.g.
// "docker/Dockerfile.prod") is still excluded by its basename: the ignore
// matcher only ever receives filepath.Base(path), so appending the raw
// service.Build.Dockerfile value (which may include the subdirectory) would
// never match.
func TestInitialSync_ExcludesNestedCustomNamedDockerfile(t *testing.T) {
	hostDir := t.TempDir()
	assert.NilError(t, os.MkdirAll(filepath.Join(hostDir, "docker"), 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(hostDir, "docker", "Dockerfile.prod"), []byte("content"), 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(hostDir, "app.go"), []byte("content"), 0o600))

	syncer := &fakeSyncer{synced: make(chan []*sync.PathMapping, 1)}
	err := (&composeService{}).initialSync(t.Context(), types.ServiceConfig{
		Name:  "svc",
		Build: &types.BuildConfig{Context: hostDir, Dockerfile: "docker/Dockerfile.prod"},
	}, types.Trigger{
		Path:   hostDir,
		Target: "/app",
	}, syncer)
	assert.NilError(t, err)

	paths := <-syncer.synced
	assert.DeepEqual(t, paths, []*sync.PathMapping{{
		HostPath:      filepath.Join(hostDir, "app.go"),
		ContainerPath: "/app/app.go",
	}})
}

// TestPruneDanglingImagesOnRebuild verifies the post-rebuild prune only
// removes superseded dangling images: a dangling image whose ID matches one
// of the freshly built images must be spared. The lookup used to probe the
// name-keyed map with an image ID, matching nothing — every dangling image
// of the project was removed on each rebuild.
func TestPruneDanglingImagesOnRebuild(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	apiMock.EXPECT().ImageList(gomock.Any(), gomock.Any()).
		Return(client.ImageListResult{Items: []image.Summary{
			{ID: "sha256:justbuilt"},
			{ID: "sha256:superseded"},
		}}, nil)
	// only the superseded image may be removed; removing the just-built one
	// would be an unexpected call and fail the test
	apiMock.EXPECT().ImageRemove(gomock.Any(), "sha256:superseded", gomock.Any()).
		Return(client.ImageRemoveResult{}, nil)

	tested.(*composeService).pruneDanglingImagesOnRebuild(t.Context(), "proj",
		map[string]string{"app-image:latest": "sha256:justbuilt"})
}
