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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

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
	Path   string `json:"path,omitempty"`
	Action string `json:"action,omitempty"`
	Target string `json:"target,omitempty"`
}

const quietPeriod = 2 * time.Second

// fileMapping contains the Compose service and modified host system path.
//
// For file sync, the container path is also included.
// For rebuild, there is no container path, so it is always empty.
type fileMapping struct {
	// service that the file event is for.
	service string
	// hostPath that was created/modified/deleted outside the container.
	//
	// This is the path as seen from the user's perspective, e.g.
	// 	- C:\Users\moby\Documents\hello-world\main.go
	//  - /Users/moby/Documents/hello-world/main.go
	hostPath string
	// containerPath for the target file inside the container (only populated
	// for sync events, not rebuild).
	//
	// This is the path as used in Docker CLI commands, e.g.
	//	- /workdir/main.go
	containerPath string
}

func (s *composeService) Watch(ctx context.Context, project *types.Project, services []string, _ api.WatchOptions) error { //nolint:gocyclo
	needRebuild := make(chan fileMapping)
	needSync := make(chan fileMapping)

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
	for _, service := range ss {
		config, err := loadDevelopmentConfig(service, project)
		if err != nil {
			return err
		}
		name := service.Name
		if service.Build == nil {
			if len(services) != 0 || len(config.Watch) != 0 {
				// watch explicitly requested on service, but no build section set
				return fmt.Errorf("service %s doesn't have a build section", name)
			}
			logrus.Infof("service %s ignored. Can't watch a service without a build section", name)
			continue
		}
		bc := service.Build.Context

		dockerIgnores, err := watch.LoadDockerIgnore(bc)
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
			watch.EphemeralPathMatcher,
			dotGitIgnore,
		)

		watcher, err := watch.NewWatcher([]string{bc}, ignore)
		if err != nil {
			return err
		}

		fmt.Fprintf(s.stderr(), "watching %s\n", bc)
		err = watcher.Start()
		if err != nil {
			return err
		}

		eg.Go(func() error {
			defer watcher.Close() //nolint:errcheck
		WATCH:
			for {
				select {
				case <-ctx.Done():
					return nil
				case event := <-watcher.Events():
					hostPath := event.Path()

					for _, trigger := range config.Watch {
						logrus.Debugf("change detected on %s - comparing with %s", hostPath, trigger.Path)
						if watch.IsChild(trigger.Path, hostPath) {
							fmt.Fprintf(s.stderr(), "change detected on %s\n", hostPath)

							f := fileMapping{
								hostPath: hostPath,
								service:  name,
							}

							switch trigger.Action {
							case WatchActionSync:
								logrus.Debugf("modified file %s triggered sync", hostPath)
								rel, err := filepath.Rel(trigger.Path, hostPath)
								if err != nil {
									return err
								}
								// always use Unix-style paths for inside the container
								f.containerPath = path.Join(trigger.Target, rel)
								needSync <- f
							case WatchActionRebuild:
								logrus.Debugf("modified file %s requires image to be rebuilt", hostPath)
								needRebuild <- f
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
		})
	}

	return eg.Wait()
}

func loadDevelopmentConfig(service types.ServiceConfig, project *types.Project) (DevelopmentConfig, error) {
	var config DevelopmentConfig
	if y, ok := service.Extensions["x-develop"]; ok {
		err := mapstructure.Decode(y, &config)
		if err != nil {
			return config, err
		}
		for i, trigger := range config.Watch {
			if !filepath.IsAbs(trigger.Path) {
				trigger.Path = filepath.Join(project.WorkingDir, trigger.Path)
			}
			trigger.Path = filepath.Clean(trigger.Path)
			if trigger.Path == "" {
				return config, errors.New("watch rules MUST define a path")
			}
			config.Watch[i] = trigger
		}
	}
	return config, nil
}

func (s *composeService) makeRebuildFn(ctx context.Context, project *types.Project) func(services rebuildServices) {
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
			s.stderr(),
			"Rebuilding %s after changes were detected:%s\n",
			strings.Join(serviceNames, ", "),
			strings.Join(append([]string{""}, allPaths.Elements()...), "\n  - "),
		)
		imageIds, err := s.build(ctx, project, api.BuildOptions{
			Services: serviceNames,
		})
		if err != nil {
			fmt.Fprintf(s.stderr(), "Build failed\n")
		}
		for i, service := range project.Services {
			if id, ok := imageIds[service.Name]; ok {
				service.Image = id
			}
			project.Services[i] = service
		}

		err = s.Up(ctx, project, api.UpOptions{
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

func (s *composeService) makeSyncFn(ctx context.Context, project *types.Project, needSync <-chan fileMapping) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case opt := <-needSync:
				if fi, statErr := os.Stat(opt.hostPath); statErr == nil && !fi.IsDir() {
					err := s.Copy(ctx, project.Name, api.CopyOptions{
						Source:      opt.hostPath,
						Destination: fmt.Sprintf("%s:%s", opt.service, opt.containerPath),
					})
					if err != nil {
						return err
					}
					fmt.Fprintf(s.stderr(), "%s updated\n", opt.containerPath)
				} else if errors.Is(statErr, fs.ErrNotExist) {
					_, err := s.Exec(ctx, project.Name, api.RunOptions{
						Service: opt.service,
						Command: []string{"rm", "-rf", opt.containerPath},
						Index:   1,
					})
					if err != nil {
						logrus.Warnf("failed to delete %q from %s: %v", opt.containerPath, opt.service, err)
					}
					fmt.Fprintf(s.stderr(), "%s deleted from container\n", opt.containerPath)
				}
			}
		}
	}
}

type rebuildServices map[string]utils.Set[string]

func debounce(ctx context.Context, clock clockwork.Clock, delay time.Duration, input <-chan fileMapping, fn func(services rebuildServices)) {
	services := make(rebuildServices)
	t := clock.AfterFunc(delay, func() {
		if len(services) > 0 {
			fn(services)
			// TODO(milas): this is a data race!
			services = make(rebuildServices)
		}
	})
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-input:
			t.Reset(delay)
			svc, ok := services[e.service]
			if !ok {
				svc = make(utils.Set[string])
				services[e.service] = svc
			}
			svc.Add(e.hostPath)
		}
	}
}
