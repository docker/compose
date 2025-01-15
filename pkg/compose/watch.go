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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/compose-spec/compose-go/v2/utils"
	ccli "github.com/docker/cli/cli/command/container"
	pathutil "github.com/docker/compose/v2/internal/paths"
	"github.com/docker/compose/v2/internal/sync"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/watch"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

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

type watchRule struct {
	types.Trigger
	ignore  watch.PathMatcher
	service string
}

func (r watchRule) Matches(event watch.FileEvent) *sync.PathMapping {
	hostPath := string(event)
	if !pathutil.IsChild(r.Path, hostPath) {
		return nil
	}
	isIgnored, err := r.ignore.Matches(hostPath)
	if err != nil {
		logrus.Warnf("error ignore matching %q: %v", hostPath, err)
		return nil
	}

	if isIgnored {
		logrus.Debugf("%s is matching ignore pattern", hostPath)
		return nil
	}

	var containerPath string
	if r.Target != "" {
		rel, err := filepath.Rel(r.Path, hostPath)
		if err != nil {
			logrus.Warnf("error making %s relative to %s: %v", hostPath, r.Path, err)
			return nil
		}
		// always use Unix-style paths for inside the container
		containerPath = path.Join(r.Target, filepath.ToSlash(rel))
	}
	return &sync.PathMapping{
		HostPath:      hostPath,
		ContainerPath: containerPath,
	}
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
	options.LogTo.Register(api.WatchLogger)

	var (
		rules []watchRule
		paths []string
	)
	for serviceName, service := range project.Services {
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
				// set the service to always be built - watch triggers `Up()` when it receives a rebuild event
				service.PullPolicy = types.PullPolicyBuild
				project.Services[serviceName] = service
			}
		}

		for _, trigger := range config.Watch {
			if isSync(trigger) && checkIfPathAlreadyBindMounted(trigger.Path, service.Volumes) {
				logrus.Warnf("path '%s' also declared by a bind mount volume, this path won't be monitored!\n", trigger.Path)
				continue
			} else {
				var initialSync bool
				success, err := trigger.Extensions.Get("x-initialSync", &initialSync)
				if err == nil && success && initialSync && isSync(trigger) {
					// Need to check initial files are in container that are meant to be synched from watch action
					err := s.initialSync(ctx, project, service, trigger, syncer)
					if err != nil {
						return err
					}
				}
			}
			paths = append(paths, trigger.Path)
		}

		serviceWatchRules, err := getWatchRules(config, service)
		if err != nil {
			return err
		}
		rules = append(rules, serviceWatchRules...)
	}

	if len(paths) == 0 {
		return fmt.Errorf("none of the selected services is configured for watch, consider setting an 'develop' section")
	}

	watcher, err := watch.NewWatcher(paths)
	if err != nil {
		return err
	}

	err = watcher.Start()
	if err != nil {
		return err
	}

	defer func() {
		if err := watcher.Close(); err != nil {
			logrus.Debugf("Error closing watcher: %v", err)
		}
	}()

	eg.Go(func() error {
		return s.watchEvents(ctx, project, options, watcher, syncer, rules)
	})
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

func getWatchRules(config *types.DevelopConfig, service types.ServiceConfig) ([]watchRule, error) {
	var rules []watchRule

	dockerIgnores, err := watch.LoadDockerIgnore(service.Build)
	if err != nil {
		return nil, err
	}

	// add a hardcoded set of ignores on top of what came from .dockerignore
	// some of this should likely be configurable (e.g. there could be cases
	// where you want `.git` to be synced) but this is suitable for now
	dotGitIgnore, err := watch.NewDockerPatternMatcher("/", []string{".git/"})
	if err != nil {
		return nil, err
	}

	for _, trigger := range config.Watch {
		ignore, err := watch.NewDockerPatternMatcher(trigger.Path, trigger.Ignore)
		if err != nil {
			return nil, err
		}

		rules = append(rules, watchRule{
			Trigger: trigger,
			ignore: watch.NewCompositeMatcher(
				dockerIgnores,
				watch.EphemeralPathMatcher(),
				dotGitIgnore,
				ignore,
			),
			service: service.Name,
		})
	}
	return rules, nil
}

func isSync(trigger types.Trigger) bool {
	return trigger.Action == types.WatchActionSync || trigger.Action == types.WatchActionSyncRestart
}

func (s *composeService) watchEvents(ctx context.Context, project *types.Project, options api.WatchOptions, watcher watch.Notify, syncer sync.Syncer, rules []watchRule) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// debounce and group filesystem events so that we capture IDE saving many files as one "batch" event
	batchEvents := watch.BatchDebounceEvents(ctx, s.clock, watcher.Events())

	for {
		select {
		case <-ctx.Done():
			options.LogTo.Log(api.WatchLogger, "Watch disabled")
			return nil
		case err := <-watcher.Errors():
			options.LogTo.Err(api.WatchLogger, "Watch disabled with errors")
			return err
		case batch := <-batchEvents:
			start := time.Now()
			logrus.Debugf("batch start: count[%d]", len(batch))
			err := s.handleWatchBatch(ctx, project, options, batch, rules, syncer)
			if err != nil {
				logrus.Warnf("Error handling changed files: %v", err)
			}
			logrus.Debugf("batch complete: duration[%s] count[%d]", time.Since(start), len(batch))
		}
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
			return nil, fmt.Errorf("service %s doesn't have a build section, can't apply %s on watch", types.WatchActionRebuild, service.Name)
		}
		if trigger.Action == types.WatchActionSyncExec && len(trigger.Exec.Command) == 0 {
			return nil, fmt.Errorf("can't watch with action %q on service %s wihtout a command", types.WatchActionSyncExec, service.Name)
		}

		config.Watch[i] = trigger
	}
	return &config, nil
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

//nolint:gocyclo
func (s *composeService) handleWatchBatch(ctx context.Context, project *types.Project, options api.WatchOptions, batch []watch.FileEvent, rules []watchRule, syncer sync.Syncer) error {
	var (
		restart   = map[string]bool{}
		syncfiles = map[string][]*sync.PathMapping{}
		exec      = map[string][]int{}
		rebuild   = map[string]bool{}
	)
	for _, event := range batch {
		for i, rule := range rules {
			mapping := rule.Matches(event)
			if mapping == nil {
				continue
			}

			switch rule.Action {
			case types.WatchActionRebuild:
				rebuild[rule.service] = true
			case types.WatchActionSync:
				syncfiles[rule.service] = append(syncfiles[rule.service], mapping)
			case types.WatchActionRestart:
				restart[rule.service] = true
			case types.WatchActionSyncRestart:
				syncfiles[rule.service] = append(syncfiles[rule.service], mapping)
				restart[rule.service] = true
			case types.WatchActionSyncExec:
				syncfiles[rule.service] = append(syncfiles[rule.service], mapping)
				// We want to run exec hooks only once after syncfiles if multiple file events match
				// as we can't compare ServiceHook to sort and compact a slice, collect rule indexes
				exec[rule.service] = append(exec[rule.service], i)
			}
		}
	}

	logrus.Debugf("watch actions: rebuild %d sync %d restart %d", len(rebuild), len(syncfiles), len(restart))

	if len(rebuild) > 0 {
		err := s.rebuild(ctx, project, utils.MapKeys(rebuild), options)
		if err != nil {
			return err
		}
	}

	for serviceName, pathMappings := range syncfiles {
		writeWatchSyncMessage(options.LogTo, serviceName, pathMappings)
		err := syncer.Sync(ctx, serviceName, pathMappings)
		if err != nil {
			return err
		}
	}
	if len(restart) > 0 {
		services := utils.MapKeys(restart)
		err := s.restart(ctx, project.Name, api.RestartOptions{
			Services: services,
			Project:  project,
			NoDeps:   false,
		})
		if err != nil {
			return err
		}
		options.LogTo.Log(
			api.WatchLogger,
			fmt.Sprintf("service(s) %q restarted", services))
	}

	eg, ctx := errgroup.WithContext(ctx)
	for service, rulesToExec := range exec {
		slices.Sort(rulesToExec)
		for _, i := range slices.Compact(rulesToExec) {
			err := s.exec(ctx, project, service, rules[i].Exec, eg)
			if err != nil {
				return err
			}
		}
	}
	return eg.Wait()
}

func (s *composeService) exec(ctx context.Context, project *types.Project, serviceName string, x types.ServiceHook, eg *errgroup.Group) error {
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, false, serviceName)
	if err != nil {
		return err
	}
	for _, c := range containers {
		eg.Go(func() error {
			exec := ccli.NewExecOptions()
			exec.User = x.User
			exec.Privileged = x.Privileged
			exec.Command = x.Command
			exec.Workdir = x.WorkingDir
			for _, v := range x.Environment.ToMapping().Values() {
				err := exec.Env.Set(v)
				if err != nil {
					return err
				}
			}
			return ccli.RunExec(ctx, s.dockerCli, c.ID, exec)
		})
	}
	return nil
}

func (s *composeService) rebuild(ctx context.Context, project *types.Project, services []string, options api.WatchOptions) error {
	options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Rebuilding service(s) %q after changes were detected...", services))
	// restrict the build to ONLY this service, not any of its dependencies
	options.Build.Services = services
	imageNameToIdMap, err := s.build(ctx, project, *options.Build, nil)
	if err != nil {
		options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Build failed. Error: %v", err))
		return err
	}

	if options.Prune {
		s.pruneDanglingImagesOnRebuild(ctx, project.Name, imageNameToIdMap)
	}

	options.LogTo.Log(api.WatchLogger, fmt.Sprintf("service(s) %q successfully built", services))

	err = s.create(ctx, project, api.CreateOptions{
		Services: services,
		Inherit:  true,
		Recreate: api.RecreateForce,
	})
	if err != nil {
		options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Failed to recreate services after update. Error: %v", err))
		return err
	}

	p, err := project.WithSelectedServices(services)
	if err != nil {
		return err
	}
	err = s.start(ctx, project.Name, api.StartOptions{
		Project:  p,
		Services: services,
		AttachTo: services,
	}, nil)
	if err != nil {
		options.LogTo.Log(api.WatchLogger, fmt.Sprintf("Application failed to start after update. Error: %v", err))
	}
	return nil
}

// writeWatchSyncMessage prints out a message about the sync for the changed paths.
func writeWatchSyncMessage(log api.LogConsumer, serviceName string, pathMappings []*sync.PathMapping) {
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		hostPathsToSync := make([]string, len(pathMappings))
		for i := range pathMappings {
			hostPathsToSync[i] = pathMappings[i].HostPath
		}
		log.Log(
			api.WatchLogger,
			fmt.Sprintf(
				"Syncing service %q after changes were detected: %s",
				serviceName,
				strings.Join(hostPathsToSync, ", "),
			),
		)
	} else {
		log.Log(
			api.WatchLogger,
			fmt.Sprintf("Syncing service %q after %d changes were detected", serviceName, len(pathMappings)),
		)
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

// Walks develop.watch.path and checks which files should be copied inside the container
// ignores develop.watch.ignore, Dockerfile, compose files, bind mounted paths and .git
func (s *composeService) initialSync(ctx context.Context, project *types.Project, service types.ServiceConfig, trigger types.Trigger, syncer sync.Syncer) error {
	dockerIgnores, err := watch.LoadDockerIgnore(service.Build)
	if err != nil {
		return err
	}

	dotGitIgnore, err := watch.NewDockerPatternMatcher("/", []string{".git/"})
	if err != nil {
		return err
	}

	triggerIgnore, err := watch.NewDockerPatternMatcher(trigger.Path, trigger.Ignore)
	if err != nil {
		return err
	}
	// FIXME .dockerignore
	ignoreInitialSync := watch.NewCompositeMatcher(
		dockerIgnores,
		watch.EphemeralPathMatcher(),
		dotGitIgnore,
		triggerIgnore)

	pathsToCopy, err := s.initialSyncFiles(ctx, project, service, trigger, ignoreInitialSync)
	if err != nil {
		return err
	}

	return syncer.Sync(ctx, service.Name, pathsToCopy)
}

// Syncs files from develop.watch.path if thy have been modified after the image has been created
//
//nolint:gocyclo
func (s *composeService) initialSyncFiles(ctx context.Context, project *types.Project, service types.ServiceConfig, trigger types.Trigger, ignore watch.PathMatcher) ([]*sync.PathMapping, error) {
	fi, err := os.Stat(trigger.Path)
	if err != nil {
		return nil, err
	}
	timeImageCreated, err := s.imageCreatedTime(ctx, project, service.Name)
	if err != nil {
		return nil, err
	}
	var pathsToCopy []*sync.PathMapping
	switch mode := fi.Mode(); {
	case mode.IsDir():
		// process directory
		err = filepath.WalkDir(trigger.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// handle possible path err, just in case...
				return err
			}
			if trigger.Path == path {
				// walk starts at the root directory
				return nil
			}
			if shouldIgnore(filepath.Base(path), ignore) || checkIfPathAlreadyBindMounted(path, service.Volumes) {
				// By definition sync ignores bind mounted paths
				if d.IsDir() {
					// skip folder
					return fs.SkipDir
				}
				return nil // skip file
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !d.IsDir() {
				if info.ModTime().Before(timeImageCreated) {
					// skip file if it was modified before image creation
					return nil
				}
				rel, err := filepath.Rel(trigger.Path, path)
				if err != nil {
					return err
				}
				// only copy files (and not full directories)
				pathsToCopy = append(pathsToCopy, &sync.PathMapping{
					HostPath:      path,
					ContainerPath: filepath.Join(trigger.Target, rel),
				})
			}
			return nil
		})
	case mode.IsRegular():
		// process file
		if fi.ModTime().After(timeImageCreated) && !shouldIgnore(filepath.Base(trigger.Path), ignore) && !checkIfPathAlreadyBindMounted(trigger.Path, service.Volumes) {
			pathsToCopy = append(pathsToCopy, &sync.PathMapping{
				HostPath:      trigger.Path,
				ContainerPath: trigger.Target,
			})
		}
	}
	return pathsToCopy, err
}

func shouldIgnore(name string, ignore watch.PathMatcher) bool {
	shouldIgnore, _ := ignore.Matches(name)
	// ignore files that match any ignore pattern
	return shouldIgnore
}

// gets the image creation time for a service
func (s *composeService) imageCreatedTime(ctx context.Context, project *types.Project, serviceName string) (time.Time, error) {
	containers, err := s.apiClient().ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, project.Name)),
			filters.Arg("label", fmt.Sprintf("%s=%s", api.ServiceLabel, serviceName))),
	})
	if err != nil {
		return time.Now(), err
	}
	if len(containers) == 0 {
		return time.Now(), fmt.Errorf("Could not get created time for service's image")
	}

	img, _, err := s.apiClient().ImageInspectWithRaw(ctx, containers[0].ImageID)
	if err != nil {
		return time.Now(), err
	}
	// Need to get oldest one?
	timeCreated, err := time.Parse(time.RFC3339Nano, img.Created)
	if err != nil {
		return time.Now(), err
	}
	return timeCreated, nil
}
