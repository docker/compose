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

package devwatch

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/internal/sync"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/watch"
)

// fakeBackend stands in for the engine: the calls a test does not expect fail
// it, the ones it does are served by the func fields.
type fakeBackend struct {
	t *testing.T

	build                func(ctx context.Context, project *types.Project, options api.BuildOptions) (map[string]string, error)
	createAndStart       func(ctx context.Context, project *types.Project, options api.CreateOptions) error
	removeDanglingImages func(ctx context.Context, projectName string, keep func(image.Summary) bool) error
}

func (f *fakeBackend) APIClient() client.APIClient {
	f.t.Errorf("unexpected call to the Docker client")
	return nil
}

func (f *fakeBackend) MaxConcurrency() int {
	return 0
}

func (f *fakeBackend) CLI() command.Cli {
	f.t.Errorf("unexpected call to the Docker CLI")
	return nil
}

func (f *fakeBackend) Containers(context.Context, string, engine.OneOff, bool, ...string) ([]container.Summary, error) {
	f.t.Errorf("unexpected container listing")
	return nil, nil
}

func (f *fakeBackend) SpecifiedContainer(context.Context, string, engine.OneOff, bool, string, int) (container.Summary, error) {
	f.t.Errorf("unexpected container lookup")
	return container.Summary{}, nil
}

func (f *fakeBackend) Build(ctx context.Context, project *types.Project, options api.BuildOptions) (map[string]string, error) {
	if f.build == nil {
		f.t.Errorf("unexpected build")
		return nil, nil
	}
	return f.build(ctx, project, options)
}

func (f *fakeBackend) CreateAndStart(ctx context.Context, project *types.Project, options api.CreateOptions) error {
	if f.createAndStart == nil {
		f.t.Errorf("unexpected create and start")
		return nil
	}
	return f.createAndStart(ctx, project, options)
}

func (f *fakeBackend) Restart(context.Context, string, api.RestartOptions) error {
	f.t.Errorf("unexpected restart")
	return nil
}

func (f *fakeBackend) RemoveDanglingImages(ctx context.Context, projectName string, keep func(image.Summary) bool) error {
	if f.removeDanglingImages == nil {
		f.t.Errorf("unexpected image removal")
		return nil
	}
	return f.removeDanglingImages(ctx, projectName, keep)
}

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
	synctest.Test(t, func(t *testing.T) {
		// we expect the image to be pruned
		var pruned atomic.Int32
		backend := &fakeBackend{
			t: t,
			build: func(context.Context, *types.Project, api.BuildOptions) (map[string]string, error) {
				return map[string]string{}, nil
			},
			createAndStart: func(_ context.Context, _ *types.Project, options api.CreateOptions) error {
				assert.DeepEqual(t, options, api.CreateOptions{
					Services:      []string{"test"},
					Inherit:       true,
					Recreate:      api.RecreateForce,
					SkipProviders: true,
				})
				return nil
			},
			removeDanglingImages: func(_ context.Context, projectName string, keep func(image.Summary) bool) error {
				assert.Equal(t, projectName, "myProjectName")
				// nothing was just built: no dangling image is spared
				assert.Assert(t, !keep(image.Summary{ID: "123"}))
				assert.Assert(t, !keep(image.Summary{ID: "456"}))
				pruned.Add(1)
				return nil
			},
		}

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
		go func() {
			service := New(backend)
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

			err = service.watchEvents(t.Context(), &proj, api.WatchOptions{
				Build: &api.BuildOptions{},
				LogTo: stdLogger{},
				Prune: true,
			}, watcher, syncer, rules)
			assert.NilError(t, err)
		}()

		watcher.Events() <- watch.NewFileEvent("/sync/changed")
		watcher.Events() <- watch.NewFileEvent("/sync/changed/sub")
		time.Sleep(watch.QuietPeriod)
		synctest.Wait()
		actual := <-syncer.synced
		expected := []*sync.PathMapping{
			{HostPath: "/sync/changed", ContainerPath: "/work/changed"},
			{HostPath: "/sync/changed/sub", ContainerPath: "/work/changed/sub"},
		}
		slices.SortFunc(actual, func(a, b *sync.PathMapping) int {
			return cmp.Compare(a.HostPath, b.HostPath)
		})
		assert.DeepEqual(t, expected, actual)

		// The rebuild triggered by "/rebuild" now runs asynchronously, so it no
		// longer blocks the sync of "/sync/changed" from the same batch.
		// synctest.Wait() only returns once the rebuild's goroutine has
		// settled (it runs to completion here, exercising the mocked
		// image prune call), so the expectation above is already satisfied by
		// the time we get here.
		watcher.Events() <- watch.NewFileEvent("/rebuild")
		watcher.Events() <- watch.NewFileEvent("/sync/changed")
		time.Sleep(watch.QuietPeriod)
		synctest.Wait()
		actual = <-syncer.synced
		expected = []*sync.PathMapping{
			{HostPath: "/sync/changed", ContainerPath: "/work/changed"},
		}
		assert.DeepEqual(t, expected, actual)
		assert.Equal(t, pruned.Load(), int32(1))
	})
}

// #14362: the .git ignore rule only matched /.git, not .git inside the watched path.
func TestWatchRules_IgnoreGitDirectoryAtAnyDepth(t *testing.T) {
	root := t.TempDir()
	rules, err := getWatchRules(&types.DevelopConfig{
		Watch: []types.Trigger{{Path: root, Action: "rebuild"}},
	}, types.ServiceConfig{Name: "test"})
	assert.NilError(t, err)
	assert.Equal(t, len(rules), 1)

	for _, tt := range []struct {
		path    string
		ignored bool
	}{
		{path: ".git", ignored: true},
		{path: ".git/index", ignored: true},
		{path: ".git/objects/ab/cdef", ignored: true},
		{path: "vendor/lib/.git/HEAD", ignored: true},
		{path: "main.go", ignored: false},
		{path: ".gitignore", ignored: false},
		{path: ".github/workflows/ci.yml", ignored: false},
	} {
		t.Run(tt.path, func(t *testing.T) {
			mapping := rules[0].Matches(watch.NewFileEvent(filepath.Join(root, filepath.FromSlash(tt.path))))
			assert.Equal(t, mapping == nil, tt.ignored)
		})
	}
}

// A rebuild running in the background from an earlier batch must not be
// raced by a plain restart of the same service triggered by a later batch:
// the restart is folded into the scheduler instead — the stale run is
// interrupted and a fresh rebuild (recreate + start, the restart intent)
// converges on a context snapshot taken after the change.
func TestHandleWatchBatch_RestartDuringInFlightRebuildConverges(t *testing.T) {
	// A correctly folded restart never touches the engine at all: any call to
	// the backend fails the test.
	service := New(&fakeBackend{t: t})

	proj := types.Project{
		Name: "myProjectName",
		Services: types.Services{
			"test": {Name: "test"},
		},
	}

	rules, err := getWatchRules(&types.DevelopConfig{
		Watch: []types.Trigger{
			{Path: "/restart", Action: "restart"},
		},
	}, types.ServiceConfig{Name: "test"})
	assert.NilError(t, err)

	var runs int32
	started := make(chan struct{}, 2)
	release := make(chan error)
	scheduler := newRebuildScheduler(t.Context(), func(ctx context.Context, _ []string) error {
		n := atomic.AddInt32(&runs, 1)
		started <- struct{}{}
		if n == 1 {
			// the stale run only ever ends by interruption
			<-ctx.Done()
			return ctx.Err()
		}
		return <-release
	})
	scheduler.Request([]string{"test"}) // simulate a rebuild still running from an earlier batch
	<-started

	err = service.handleWatchBatch(t.Context(), &proj, api.WatchOptions{LogTo: stdLogger{}},
		[]watch.FileEvent{watch.NewFileEvent("/restart")}, rules, newFakeSyncer(), scheduler)
	assert.NilError(t, err)

	// the trailing rebuild starting at all proves the stale run was
	// interrupted and the restart folded into a fresh rebuild
	<-started
	release <- nil
	scheduler.Wait()
	assert.Equal(t, atomic.LoadInt32(&runs), int32(2))
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

	paths, err := (&Service{}).initialSyncFiles(types.ServiceConfig{Name: "svc"}, types.Trigger{
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
	err := (&Service{}).initialSync(t.Context(), types.ServiceConfig{
		Name: "svc", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: hostDir}},
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
	err := (&Service{}).initialSync(t.Context(), types.ServiceConfig{
		Name:         "svc",
		WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: hostDir}},
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
	err := (&Service{}).initialSync(t.Context(), types.ServiceConfig{
		Name:         "svc",
		WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: hostDir, Dockerfile: "Dockerfile.prod"}},
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
	err := (&Service{}).initialSync(t.Context(), types.ServiceConfig{
		Name:         "svc",
		WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: hostDir, Dockerfile: "docker/Dockerfile.prod"}},
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
	// the images the prune is handed to spare, as the dangling ones are
	// listed: only the superseded image may be removed; removing the
	// just-built one would fail the test
	var removed []string
	tested := New(&fakeBackend{t: t, removeDanglingImages: func(_ context.Context, projectName string, keep func(image.Summary) bool) error {
		assert.Equal(t, projectName, "proj")
		for _, img := range []image.Summary{{ID: "sha256:justbuilt"}, {ID: "sha256:superseded"}} {
			if !keep(img) {
				removed = append(removed, img.ID)
			}
		}
		return nil
	}})

	tested.pruneDanglingImagesOnRebuild(t.Context(), "proj",
		map[string]string{"app-image:latest": "sha256:justbuilt"})
	assert.DeepEqual(t, removed, []string{"sha256:superseded"})
}

// TestWatcher_StartStopIndependent guards that the Start/Stop lock is per
// watcher: a Start blocked inside its WatchFunc must not stall another watcher.
func TestWatcher_StartStopIndependent(t *testing.T) {
	project := &types.Project{Services: types.Services{"app": {
		Name:    "app",
		Develop: &types.DevelopConfig{Watch: []types.Trigger{{Path: ".", Action: types.WatchActionSync}}},
	}}}

	release := make(chan struct{})
	entered := make(chan struct{})
	blocking, err := NewWatcher(project, api.UpOptions{}, func(ctx context.Context, _ *types.Project, _ api.WatchOptions) (func() error, error) {
		close(entered)
		<-release
		return func() error { return nil }, nil
	}, nil)
	assert.NilError(t, err)
	go func() { _ = blocking.Start(t.Context()) }()
	<-entered

	other, err := NewWatcher(project, api.UpOptions{}, func(ctx context.Context, _ *types.Project, _ api.WatchOptions) (func() error, error) {
		return func() error { return nil }, nil
	}, nil)
	assert.NilError(t, err)

	done := make(chan error, 1)
	go func() {
		if err := other.Start(t.Context()); err != nil {
			done <- err
			return
		}
		done <- other.Stop()
	}()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a Watcher blocked on another Watcher's Start: the lock must not be shared")
	}
	close(release)
	assert.NilError(t, blocking.Stop())
}

// A sync trigger writes into the container: when its path is already
// bind-mounted the sync writes back to the host, which the watcher sees as a
// new change, looping forever. Every action that syncs must be skipped; the
// others (restart, rebuild) don't write and stay monitored.
func TestWatchTriggerPaths_BindMountedPath(t *testing.T) {
	hostDir := t.TempDir()
	service := types.ServiceConfig{
		Name: "svc",
		ContainerSpec: types.ContainerSpec{
			Volumes: []types.ServiceVolumeConfig{
				{Type: types.VolumeTypeBind, Source: hostDir, Target: "/code", Bind: &types.ServiceVolumeBind{}},
			},
		},
	}

	for action, monitored := range map[types.WatchAction]bool{
		types.WatchActionSync:        false,
		types.WatchActionSyncRestart: false,
		types.WatchActionSyncExec:    false,
		types.WatchActionRestart:     true,
		types.WatchActionRebuild:     true,
	} {
		t.Run(string(action), func(t *testing.T) {
			paths, err := (&Service{}).watchTriggerPaths(t.Context(), service, &types.DevelopConfig{
				Watch: []types.Trigger{{Path: hostDir, Action: action, Target: "/tmp/target"}},
			}, nil)
			assert.NilError(t, err)
			if monitored {
				assert.DeepEqual(t, paths, []string{hostDir})
			} else {
				assert.Equal(t, len(paths), 0)
			}
		})
	}
}

// initial_sync applies to every sync+x action, sync+exec included.
func TestWatchTriggerPaths_InitialSyncExec(t *testing.T) {
	hostDir := t.TempDir()
	hostFile := filepath.Join(hostDir, "test.txt")
	assert.NilError(t, os.WriteFile(hostFile, []byte("hello"), 0o600))

	syncer := &fakeSyncer{synced: make(chan []*sync.PathMapping, 1)}
	paths, err := (&Service{}).watchTriggerPaths(t.Context(), types.ServiceConfig{
		Name: "svc", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: hostDir}},
	}, &types.DevelopConfig{
		Watch: []types.Trigger{{Path: hostDir, Action: types.WatchActionSyncExec, Target: "/app", InitialSync: true}},
	}, syncer)
	assert.NilError(t, err)
	assert.DeepEqual(t, paths, []string{hostDir})
	select {
	case synced := <-syncer.synced:
		assert.DeepEqual(t, synced, []*sync.PathMapping{{
			HostPath:      hostFile,
			ContainerPath: "/app/test.txt",
		}})
	default:
		t.Fatal("initial_sync was not performed for sync+exec")
	}
}
