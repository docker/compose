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
	"path/filepath"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/compose/v2/pkg/watch"
	"github.com/jonboulle/clockwork"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
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

func (s *composeService) Watch(ctx context.Context, project *types.Project, services []string, options api.WatchOptions) error {
	needRebuild := make(chan string)
	needSync := make(chan api.CopyOptions, 5)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		clock := clockwork.NewRealClock()
		debounce(ctx, clock, quietPeriod, needRebuild, s.makeRebuildFn(ctx, project))
		return nil
	})

	eg.Go(s.makeSyncFn(ctx, project, needSync))

	err := project.WithServices(services, func(service types.ServiceConfig) error {
		config, err := loadDevelopmentConfig(service, project)
		if err != nil {
			return err
		}
		if service.Build == nil {
			return errors.New("can't watch a service without a build section")
		}
		context := service.Build.Context

		ignore, err := watch.LoadDockerIgnore(context)
		if err != nil {
			return err
		}

		watcher, err := watch.NewWatcher([]string{context}, ignore)
		if err != nil {
			return err
		}

		fmt.Fprintf(s.stderr(), "watching %s\n", context)
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
					path := event.Path()

					for _, trigger := range config.Watch {
						logrus.Debugf("change deteced on %s - comparing with %s", path, trigger.Path)
						if watch.IsChild(trigger.Path, path) {
							fmt.Fprintf(s.stderr(), "change detected on %s\n", path)

							switch trigger.Action {
							case WatchActionSync:
								logrus.Debugf("modified file %s triggered sync", path)
								rel, err := filepath.Rel(trigger.Path, path)
								if err != nil {
									return err
								}
								dest := filepath.Join(trigger.Target, rel)
								needSync <- api.CopyOptions{
									Source:      path,
									Destination: fmt.Sprintf("%s:%s", service.Name, dest),
								}
							case WatchActionRebuild:
								logrus.Debugf("modified file %s require image to be rebuilt", path)
								needRebuild <- service.Name
							default:
								return fmt.Errorf("watch action %q is not supported", trigger)
							}
							continue WATCH
						}
					}

					// default
					needRebuild <- service.Name

				case err := <-watcher.Errors():
					return err
				}
			}
		})
		return nil
	})
	if err != nil {
		return err
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

func (s *composeService) makeRebuildFn(ctx context.Context, project *types.Project) func(services []string) {
	return func(services []string) {
		fmt.Fprintf(s.stderr(), "Updating %s after changes were detected\n", strings.Join(services, ", "))
		imageIds, err := s.build(ctx, project, api.BuildOptions{
			Services: services,
		})
		if err != nil {
			fmt.Fprintf(s.stderr(), "Build failed")
		}
		for i, service := range project.Services {
			if id, ok := imageIds[service.Name]; ok {
				service.Image = id
			}
			project.Services[i] = service
		}

		err = s.Up(ctx, project, api.UpOptions{
			Create: api.CreateOptions{
				Services: services,
				Inherit:  true,
			},
			Start: api.StartOptions{
				Services: services,
				Project:  project,
			},
		})
		if err != nil {
			fmt.Fprintf(s.stderr(), "Application failed to start after update")
		}
	}
}

func (s *composeService) makeSyncFn(ctx context.Context, project *types.Project, needSync chan api.CopyOptions) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case opt := <-needSync:
				err := s.Copy(ctx, project.Name, opt)
				if err != nil {
					return err
				}
				fmt.Fprintf(s.stderr(), "%s updated\n", opt.Source)
			}
		}
	}
}

func debounce(ctx context.Context, clock clockwork.Clock, delay time.Duration, input chan string, fn func(services []string)) {
	services := utils.Set[string]{}
	t := clock.AfterFunc(delay, func() {
		if len(services) > 0 {
			refresh := services.Elements()
			services.Clear()
			fn(refresh)
		}
	})
	for {
		select {
		case <-ctx.Done():
			return
		case service := <-input:
			t.Reset(delay)
			services.Add(service)
		}
	}
}
