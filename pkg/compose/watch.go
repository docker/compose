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
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	pathutil "github.com/docker/compose/v2/internal/paths"
	"github.com/docker/compose/v2/internal/sync"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/watch"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/jonboulle/clockwork"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const quietPeriod = 500 * time.Millisecond

// fileEvent contains the Compose service and modified host system path.
type fileEvent struct {
	sync.PathMapping
	Action types.WatchAction
}

// getSyncImplementation returns an appropriate sync implementation for the
// project.
//
// Currently, an implementation that batches files and transfers them using
// the Moby `Untar` API.
func (s *composeService) getSyncImplementation(project *types.Project) (sync.Syncer, error) {
	var useTar bool
	if useTarEnv, ok := os.LookupEnv("COMPOSE_EXPERIMENTAL_WATCH_TAR"); ok {
		useTar, _ = strconv.ParseBool(useTarEnv)
	} else {
		useTar = true
	}
	if !useTar {
		return nil, errors.New("no available sync implementation")
	}

	return sync.NewTar(project.Name, tarDockerClient{s: s}), nil
}
func (s *composeService) shouldWatch(project *types.Project) bool {
	var shouldWatch bool
	for i := range project.Services {
		service := project.Services[i]

		if service.Develop != nil && service.Develop.Watch != nil {
			shouldWatch = true
		}
	}
	return shouldWatch
}

func (s *composeService) Watch(ctx context.Context, project *types.Project, services []string, options api.WatchOptions) error {
	return s.watch(ctx, nil, project, services, options)
}
func (s *composeService) watch(ctx context.Context, syncChannel chan bool, project *types.Project, services []string, options api.WatchOptions) error { //nolint: gocyclo
	var err error
	if project, err = project.WithSelectedServices(services); err != nil {
		return err
	}
	syncer, err := s.getSyncImplementation(project)
	if err != nil {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	watching := false
	options.LogTo.Register(api.WatchLogger)
	for i := range project.Services {
		service := project.Services[i]
		config, err := loadDevelopmentConfig(service, project)
		if err != nil {
			return err
		}

		if service.Develop != nil {
			config = service.Develop
		}

		if config == nil {
			continue
		}

		for _, trigger := range config.Watch {
			if trigger.Action == types.WatchActionRebuild {
				if service.Build == nil {
					return fmt.Errorf("can't watch service %q with action %s without a build context", service.Name, types.WatchActionRebuild)
				}
				if options.Build == nil {
					return fmt.Errorf("--no-build is incompatible with watch action %s in service %s", types.WatchActionRebuild, service.Name)
				}
			}
		}

		if len(services) > 0 && service.Build == nil {
			// service explicitly selected for watch has no build section
			return fmt.Errorf("can't watch service %q without a build context", service.Name)
		}

		if len(services) == 0 && service.Build == nil {
			continue
		}

		// set the service to always be built - watch triggers `Up()` when it receives a rebuild event
		service.PullPolicy = types.PullPolicyBuild
		project.Services[i] = service

		dockerIgnores, err := watch.LoadDockerIgnore(service.Build.Context)
		if err != nil {
			return err
		}

		// add a hardcoded set of ignores on top of what came from .dockerignore
		// some of this should likely be configurable (e.g. there could be cases
		// where you want `.git` to be synced) but this is suitable for now
		dotGitIgnore, err := watch.NewDockerPatternMatcher("/", []string{".git/"})
		if err != nil {
			return err
		}
		ignore := watch.NewCompositeMatcher(
			dockerIgnores,
			watch.EphemeralPathMatcher(),
			dotGitIgnore,
		)

		var paths, pathLogs []string
		for _, trigger := range config.Watch {
			if checkIfPathAlreadyBindMounted(trigger.Path, service.Volumes) {
				logrus.Warnf("path '%s' also declared by a bind mount volume, this path won't be monitored!\n", trigger.Path)
				continue
			}
			paths = append(paths, trigger.Path)
			pathLogs = append(pathLogs, fmt.Sprintf("Action %s for path %q", trigger.Action, trigger.Path))
		}

		watcher, err := watch.NewWatcher(paths, ignore)
		if err != nil {
			return err
		}

		logrus.Debugf("Watch configuration for service %q:%s\n",
			service.Name,
			strings.Join(append([]string{""}, pathLogs...), "\n  - "),
		)
		err = watcher.Start()
		if err != nil {
			return err
		}
		watching = true
		eg.Go(func() error {
			defer func() {
				if err := watcher.Close(); err != nil {
					logrus.Debugf("Error closing watcher for service %s: %v", service.Name, err)
				}
			}()
			return s.watchEvents(ctx, project, service.Name, options, watcher, syncer, config.Watch)
		})
	}
	if !watching {
		return fmt.Errorf("none of the selected services is configured for watch, consider setting an 'develop' section")
	}
	options.LogTo.Log(api.WatchLogger, "Watch enabled")

	for {
		select {
		case <-ctx.Done():
			return eg.Wait()
		case <-syncChannel:
			options.LogTo.Log(api.WatchLogger, "Watch disabled")
			return nil
		}
	}
}

func (s *composeService) watchEvents(ctx context.Context, project *types.Project, name string, options api.WatchOptions, watcher watch.Notify, syncer sync.Syncer, triggers []types.Trigger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ignores := make([]watch.PathMatcher, len(triggers))
	for i, trigger := range triggers {
		ignore, err := watch.NewDockerPatternMatcher(trigger.Path, trigger.Ignore)
		if err != nil {
			return err
		}
		ignores[i] = ignore
	}

	events := make(chan fileEvent)
	batchEvents := batchDebounceEvents(ctx, s.clock, quietPeriod, events)
	quit := make(chan bool)
	go func() {
		for {
			select {
			case <-ctx.Done():
				quit <- true
				return
			case batch := <-batchEvents:
				start := time.Now()
				logrus.Debugf("batch start: service[%s] count[%d]", name, len(batch))
				if err := s.handleWatchBatch(ctx, project, name, options, batch, syncer); err != nil {
					logrus.Warnf("Error handling changed files for service %s: %v", name, err)
				}
				logrus.Debugf("batch complete: service[%s] duration[%s] count[%d]",
					name, time.Since(start), len(batch))
			}
		}
	}()

	for {
		select {
		case <-quit:
			options.LogTo.Log(api.WatchLogger, "Watch disabled")
			return nil
		case err := <-watcher.Errors():
			options.LogTo.Err(api.WatchLogger, "Watch disabled with errors")
			return err
		case event := <-watcher.Events():
			hostPath := event.Path()
			for i, trigger := range triggers {
				logrus.Debugf("change for %s - comparing with %s", hostPath, trigger.Path)
				if fileEvent := maybeFileEvent(trigger, hostPath, ignores[i]); fileEvent != nil {
					events <- *fileEvent
				}
			}
		}
	}
}

// maybeFileEvent returns a file event object if hostPath is valid for the provided trigger and ignore
// rules.
//
// Any errors are logged as warnings and nil (no file event) is returned.
func maybeFileEvent(trigger types.Trigger, hostPath string, ignore watch.PathMatcher) *fileEvent {
	if !pathutil.IsChild(trigger.Path, hostPath) {
		return nil
	}
	isIgnored, err := ignore.Matches(hostPath)
	if err != nil {
		logrus.Warnf("error ignore matching %q: %v", hostPath, err)
		return nil
	}

	if isIgnored {
		logrus.Debugf("%s is matching ignore pattern", hostPath)
		return nil
	}

	var containerPath string
	if trigger.Target != "" {
		rel, err := filepath.Rel(trigger.Path, hostPath)
		if err != nil {
			logrus.Warnf("error making %s relative to %s: %v", hostPath, trigger.Path, err)
			return nil
		}
		// always use Unix-style paths for inside the container
		containerPath = path.Join(trigger.Target, rel)
	}

	return &fileEvent{
		Action: trigger.Action,
		PathMapping: sync.PathMapping{
			HostPath:      hostPath,
			ContainerPath: containerPath,
		},
	}
}

func loadDevelopmentConfig(service types.ServiceConfig, project *types.Project) (*types.DevelopConfig, error) {
	var config types.DevelopConfig
	y, ok := service.Extensions["x-develop"]
	if !ok {
		return nil, nil
	}
	logrus.Warnf("x-develop is DEPRECATED, please use the official `develop` attribute")
	err := mapstructure.Decode(y, &config)
	if err != nil {
		return nil, err
	}
	baseDir, err := filepath.EvalSymlinks(project.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("resolving symlink for %q: %w", project.WorkingDir, err)
	}

	for i, trigger := range config.Watch {
		if !filepath.IsAbs(trigger.Path) {
			trigger.Path = filepath.Join(baseDir, trigger.Path)
		}
		if p, err := filepath.EvalSymlinks(trigger.Path); err == nil {
			// this might fail because the path doesn't exist, etc.
			trigger.Path = p
		}
		trigger.Path = filepath.Clean(trigger.Path)
		if trigger.Path == "" {
			return nil, errors.New("watch rules MUST define a path")
		}

		if trigger.Action == types.WatchActionRebuild && service.Build == nil {
			return nil, fmt.Errorf("service %s doesn't have a build section, can't apply 'rebuild' on watch", service.Name)
		}

		config.Watch[i] = trigger
	}
	return &config, nil
}

// batchDebounceEvents groups identical file events within a sliding time window and writes the results to the returned
// channel.
//
// The returned channel is closed when the debouncer is stopped via context cancellation or by closing the input channel.
func batchDebounceEvents(ctx context.Context, clock clockwork.Clock, delay time.Duration, input <-chan fileEvent) <-chan []fileEvent {
	out := make(chan []fileEvent)
	go func() {
		defer close(out)
		seen := make(map[fileEvent]time.Time)
		flushEvents := func() {
			if len(seen) == 0 {
				return
			}
			events := make([]fileEvent, 0, len(seen))
			for e := range seen {
				events = append(events, e)
			}
			// sort batch by oldest -> newest
			// (if an event is seen > 1 per batch, it gets the latest timestamp)
			sort.SliceStable(events, func(i, j int) bool {
				x := events[i]
				y := events[j]
				return seen[x].Before(seen[y])
			})
			out <- events
			seen = make(map[fileEvent]time.Time)
		}

		t := clock.NewTicker(delay)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.Chan():
				flushEvents()
			case e, ok := <-input:
				if !ok {
					// input channel was closed
					flushEvents()
					return
				}
				seen[e] = time.Now()
				t.Reset(delay)
			}
		}
	}()
	return out
}

func checkIfPathAlreadyBindMounted(watchPath string, volumes []types.ServiceVolumeConfig) bool {
	for _, volume := range volumes {
		if volume.Bind != nil && strings.HasPrefix(watchPath, volume.Source) {
			return true
		}
	}
	return false
}

type tarDockerClient struct {
	s *composeService
}

func (t tarDockerClient) ContainersForService(ctx context.Context, projectName string, serviceName string) ([]moby.Container, error) {
	containers, err := t.s.getContainers(ctx, projectName, oneOffExclude, true, serviceName)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func (t tarDockerClient) Exec(ctx context.Context, containerID string, cmd []string, in io.Reader) error {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: false,
		AttachStderr: true,
		AttachStdin:  in != nil,
		Tty:          false,
	}
	execCreateResp, err := t.s.apiClient().ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return err
	}

	startCheck := container.ExecStartOptions{Tty: false, Detach: false}
	conn, err := t.s.apiClient().ContainerExecAttach(ctx, execCreateResp.ID, startCheck)
	if err != nil {
		return err
	}
	defer conn.Close()

	var eg errgroup.Group
	if in != nil {
		eg.Go(func() error {
			defer func() {
				_ = conn.CloseWrite()
			}()
			_, err := io.Copy(conn.Conn, in)
			return err
		})
	}
	eg.Go(func() error {
		_, err := io.Copy(t.s.stdinfo(), conn.Reader)
		return err
	})

	err = t.s.apiClient().ContainerExecStart(ctx, execCreateResp.ID, startCheck)
	if err != nil {
		return err
	}

	// although the errgroup is not tied directly to the context, the operations
	// in it are reading/writing to the connection, which is tied to the context,
	// so they won't block indefinitely
	if err := eg.Wait(); err != nil {
		return err
	}

	execResult, err := t.s.apiClient().ContainerExecInspect(ctx, execCreateResp.ID)
	if err != nil {
		return err
	}
	if execResult.Running {
		return errors.New("process still running")
	}
	if execResult.ExitCode != 0 {
		return fmt.Errorf("exit code %d", execResult.ExitCode)
	}
	return nil
}

func (t tarDockerClient) Untar(ctx context.Context, id string, archive io.ReadCloser) error {
	return t.s.apiClient().CopyToContainer(ctx, id, "/", archive, container.CopyToContainerOptions{
		CopyUIDGID: true,
	})
}

func (s *composeService) handleWatchBatch(ctx context.Context, project *types.Project, serviceName string, options api.WatchOptions, batch []fileEvent, syncer sync.Syncer) error {
	pathMappings := make([]sync.PathMapping, len(batch))
	restartService := false
	for i := range batch {
		if batch[i].Action == types.WatchActionRebuild {
			options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Rebuilding service %q after changes were detected...", serviceName))
			// restrict the build to ONLY this service, not any of its dependencies
			options.Build.Services = []string{serviceName}
			imageNameToIdMap, err := s.build(ctx, project, *options.Build, nil)

			if err != nil {
				options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Build failed. Error: %v", err))
				return err
			}

			if options.Prune {
				s.pruneDanglingImagesOnRebuild(ctx, project.Name, imageNameToIdMap)
			}

			options.LogTo.Log(api.WatchLogger, fmt.Sprintf("service %q successfully built", serviceName))

			err = s.create(ctx, project, api.CreateOptions{
				Services: []string{serviceName},
				Inherit:  true,
				Recreate: api.RecreateForce,
			})
			if err != nil {
				options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Failed to recreate service after update. Error: %v", err))
				return err
			}

			err = s.start(ctx, project.Name, api.StartOptions{
				Project:  project,
				Services: []string{serviceName},
			}, nil)
			if err != nil {
				options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Application failed to start after update. Error: %v", err))
			}
			return nil
		}
		if batch[i].Action == types.WatchActionSyncRestart {
			restartService = true
		}
		pathMappings[i] = batch[i].PathMapping
	}

	writeWatchSyncMessage(options.LogTo, serviceName, pathMappings)

	service, err := project.GetService(serviceName)
	if err != nil {
		return err
	}
	if err := syncer.Sync(ctx, service, pathMappings); err != nil {
		return err
	}
	if restartService {
		return s.Restart(ctx, project.Name, api.RestartOptions{
			Services: []string{serviceName},
			Project:  project,
			NoDeps:   false,
		})
	}
	return nil
}

// writeWatchSyncMessage prints out a message about the sync for the changed paths.
func writeWatchSyncMessage(log api.LogConsumer, serviceName string, pathMappings []sync.PathMapping) {
	const maxPathsToShow = 10
	if len(pathMappings) <= maxPathsToShow || logrus.IsLevelEnabled(logrus.DebugLevel) {
		hostPathsToSync := make([]string, len(pathMappings))
		for i := range pathMappings {
			hostPathsToSync[i] = pathMappings[i].HostPath
		}
		log.Log(api.WatchLogger, fmt.Sprintf("Syncing %q after changes were detected", serviceName))
	} else {
		hostPathsToSync := make([]string, len(pathMappings))
		for i := range pathMappings {
			hostPathsToSync[i] = pathMappings[i].HostPath
		}
		log.Log(api.WatchLogger, fmt.Sprintf("Syncing service %q after %d changes were detected", serviceName, len(pathMappings)))
	}
}

func (s *composeService) pruneDanglingImagesOnRebuild(ctx context.Context, projectName string, imageNameToIdMap map[string]string) {
	images, err := s.apiClient().ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("dangling", "true"),
			filters.Arg("label", api.ProjectLabel+"="+projectName),
		),
	})

	if err != nil {
		logrus.Debugf("Failed to list images: %v", err)
		return
	}

	for _, img := range images {
		if _, ok := imageNameToIdMap[img.ID]; !ok {
			_, err := s.apiClient().ImageRemove(ctx, img.ID, image.RemoveOptions{})
			if err != nil {
				logrus.Debugf("Failed to remove image %s: %v", img.ID, err)
			}
		}
	}
}
