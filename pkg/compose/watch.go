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
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	moby "github.com/docker/docker/api/types"

	"github.com/docker/compose/v2/internal/sync"

	"github.com/compose-spec/compose-go/types"
	"github.com/jonboulle/clockwork"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/compose/v2/pkg/watch"
)

type DevelopmentConfig struct {
	Watch []Trigger `json:"watch,omitempty"`
}

const (
	WatchActionSync    = "sync"
	WatchActionRebuild = "rebuild"
)

type Trigger struct {
	Path   string   `json:"path,omitempty"`
	Action string   `json:"action,omitempty"`
	Target string   `json:"target,omitempty"`
	Ignore []string `json:"ignore,omitempty"`
}

const quietPeriod = 2 * time.Second

// fileEvent contains the Compose service and modified host system path.
type fileEvent struct {
	// Service that the file event is for.
	Service string
	// HostPath that was created/modified/deleted outside the container.
	//
	// This is the path as seen from the user's perspective, e.g.
	// 	- C:\Users\moby\Documents\hello-world\main.go
	//  - /Users/moby/Documents/hello-world/main.go
	HostPath string
}

func (s *composeService) Watch(ctx context.Context, project *types.Project, services []string, _ api.WatchOptions) error { //nolint: gocyclo
	needRebuild := make(chan fileEvent)
	needSync := make(chan sync.PathMapping)

	_, err := s.prepareProjectForBuild(project, nil)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		clock := clockwork.NewRealClock()
		debounce(ctx, clock, quietPeriod, needRebuild, s.makeRebuildFn(ctx, project))
		return nil
	})

	eg.Go(s.makeSyncFn(ctx, project, needSync))

	ss, err := project.GetServices(services...)
	if err != nil {
		return err
	}
	watching := false
	for _, service := range ss {
		config, err := loadDevelopmentConfig(service, project)
		if err != nil {
			return err
		}

		if config == nil {
			continue
		}

		if len(config.Watch) > 0 && service.Build == nil {
			// service configured with watchers but no build section
			return fmt.Errorf("can't watch service %q without a build context", service.Name)
		}

		if len(services) > 0 && service.Build == nil {
			// service explicitly selected for watch has no build section
			return fmt.Errorf("can't watch service %q without a build context", service.Name)
		}

		if len(services) == 0 && service.Build == nil {
			continue
		}

		name := service.Name
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

		var paths []string
		for _, trigger := range config.Watch {
			if checkIfPathAlreadyBindMounted(trigger.Path, service.Volumes) {
				logrus.Warnf("path '%s' also declared by a bind mount volume, this path won't be monitored!\n", trigger.Path)
				continue
			}
			paths = append(paths, trigger.Path)
		}

		watcher, err := watch.NewWatcher(paths, ignore)
		if err != nil {
			return err
		}

		fmt.Fprintf(s.stdinfo(), "watching %s\n", paths)
		err = watcher.Start()
		if err != nil {
			return err
		}
		watching = true

		eg.Go(func() error {
			defer watcher.Close() //nolint:errcheck
			return s.watch(ctx, name, watcher, config.Watch, needSync, needRebuild)
		})
	}

	if !watching {
		return fmt.Errorf("none of the selected services is configured for watch, consider setting an 'x-develop' section")
	}

	return eg.Wait()
}

func (s *composeService) watch(ctx context.Context, name string, watcher watch.Notify, triggers []Trigger, needSync chan sync.PathMapping, needRebuild chan fileEvent) error {
	ignores := make([]watch.PathMatcher, len(triggers))
	for i, trigger := range triggers {
		ignore, err := watch.NewDockerPatternMatcher(trigger.Path, trigger.Ignore)
		if err != nil {
			return err
		}
		ignores[i] = ignore
	}

WATCH:
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-watcher.Events():
			hostPath := event.Path()

			for i, trigger := range triggers {
				logrus.Debugf("change for %s - comparing with %s", hostPath, trigger.Path)
				if watch.IsChild(trigger.Path, hostPath) {
					match, err := ignores[i].Matches(hostPath)
					if err != nil {
						logrus.Warnf("error ignore matching %q: %v", hostPath, err)
						return err
					}

					if match {
						logrus.Debugf("%s is matching ignore pattern", hostPath)
						continue
					}

					logrus.Infof("change for %q", hostPath)
					fmt.Fprintf(s.stdinfo(), "change detected on %s\n", hostPath)

					switch trigger.Action {
					case WatchActionSync:
						logrus.Debugf("modified file %s triggered sync", hostPath)
						rel, err := filepath.Rel(trigger.Path, hostPath)
						if err != nil {
							return err
						}
						needSync <- sync.PathMapping{
							Service:  name,
							HostPath: hostPath,
							// always use Unix-style paths for inside the container
							ContainerPath: path.Join(trigger.Target, rel),
						}
					case WatchActionRebuild:
						logrus.Debugf("modified file %s requires image to be rebuilt", hostPath)
						needRebuild <- fileEvent{
							HostPath: hostPath,
							Service:  name,
						}
					default:
						return fmt.Errorf("watch action %q is not supported", trigger)
					}
					continue WATCH
				}
			}
		case err := <-watcher.Errors():
			return err
		}
	}
}

func loadDevelopmentConfig(service types.ServiceConfig, project *types.Project) (*DevelopmentConfig, error) {
	var config DevelopmentConfig
	y, ok := service.Extensions["x-develop"]
	if !ok {
		return nil, nil
	}
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

		if trigger.Action == WatchActionRebuild && service.Build == nil {
			return nil, fmt.Errorf("service %s doesn't have a build section, can't apply 'rebuild' on watch", service.Name)
		}

		config.Watch[i] = trigger
	}
	return &config, nil
}

func (s *composeService) makeRebuildFn(ctx context.Context, project *types.Project) func(services rebuildServices) {
	for i, service := range project.Services {
		service.PullPolicy = types.PullPolicyBuild
		project.Services[i] = service
	}
	return func(services rebuildServices) {
		serviceNames := make([]string, 0, len(services))
		allPaths := make(utils.Set[string])
		for serviceName, paths := range services {
			serviceNames = append(serviceNames, serviceName)
			for p := range paths {
				allPaths.Add(p)
			}
		}

		fmt.Fprintf(
			s.stdinfo(),
			"Rebuilding %s after changes were detected:%s\n",
			strings.Join(serviceNames, ", "),
			strings.Join(append([]string{""}, allPaths.Elements()...), "\n  - "),
		)
		err := s.Up(ctx, project, api.UpOptions{
			Create: api.CreateOptions{
				Services: serviceNames,
				Inherit:  true,
			},
			Start: api.StartOptions{
				Services: serviceNames,
				Project:  project,
			},
		})
		if err != nil {
			fmt.Fprintf(s.stderr(), "Application failed to start after update\n")
		}
	}
}

func (s *composeService) makeSyncFn(
	ctx context.Context,
	project *types.Project,
	needSync <-chan sync.PathMapping,
) func() error {
	var syncer sync.Syncer
	if useTar, _ := strconv.ParseBool(os.Getenv("COMPOSE_EXPERIMENTAL_WATCH_TAR")); useTar {
		syncer = sync.NewTar(project.Name, tarDockerClient{s: s})
	} else {
		syncer = sync.NewDockerCopy(project.Name, s, s.stdinfo())
	}

	return func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case op := <-needSync:
				service, err := project.GetService(op.Service)
				if err != nil {
					return err
				}
				if err := syncer.Sync(ctx, service, []sync.PathMapping{op}); err != nil {
					return err
				}
			}
		}
	}
}

type rebuildServices map[string]utils.Set[string]

func debounce(ctx context.Context, clock clockwork.Clock, delay time.Duration, input <-chan fileEvent, fn func(services rebuildServices)) {
	services := make(rebuildServices)
	t := clock.NewTimer(delay)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.Chan():
			if len(services) > 0 {
				go fn(services)
				services = make(rebuildServices)
			}
		case e := <-input:
			t.Reset(delay)
			svc, ok := services[e.Service]
			if !ok {
				svc = make(utils.Set[string])
				services[e.Service] = svc
			}
			svc.Add(e.HostPath)
		}
	}
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
	execCfg := moby.ExecConfig{
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

	startCheck := moby.ExecStartCheck{Tty: false, Detach: false}
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
	if execResult.ExitCode != 0 {
		return fmt.Errorf("exit code %d", execResult.ExitCode)
	}
	return nil
}
