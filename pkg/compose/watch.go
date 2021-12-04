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
	"time"

	"github.com/docker/compose/v2/pkg/api"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
)

//nolint:gocyclo
func (s *composeService) Watch(ctx context.Context, project *types.Project, options api.WatchOptions) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, service := range project.Services {
		service := service
		if service.Build == nil {
			// ignored
			continue
		}
		path := service.Build.Context
		fmt.Printf("watching build context %s for service %s\n", path, service.Name)
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		err = watcher.Add(path)
		if err != nil {
			return err
		}
		defer watcher.Close() //nolint:errcheck

		excludes, err := build.ReadDockerignore(path)
		if err != nil {
			return err
		}
		pm, err := fileutils.NewPatternMatcher(excludes)
		if err != nil {
			return err
		}

		eg.Go(func() error {
			triggered := make(chan bool)

			// use as a guard to enforce we run a single concurrent `refresh`
			ready := make(chan bool, 1)
			ready <- true
			refresh := func() {
				select {
				case <-ready:
					eg.Go(func() error {
						triggered <- true
						err := s.refresh(ctx, project, service.Name)
						if err != nil {
							return err
						}
						ready <- true
						return nil
					})
				default:
				}
			}

			for {
				var changes []string

				select {
				case event := <-watcher.Events:
					ignore, err := pm.Matches(event.Name)
					if err != nil {
						return err
					}
					if ignore {
						continue
					}
					changes = append(changes, event.Name)
					if len(changes) == 1 {
						// change detected, trigger a refresh but apply a quiet period waiting for more changes in a row
						eg.Go(func() error {
							time.Sleep(500 * time.Millisecond)
							refresh()
							return nil
						})
					} else {
						refresh()
					}
				case <-triggered:
					// a refresh has just started, reset the pending changes list
					changes = nil
				case err := <-watcher.Errors:
					return err
				case <-ctx.Done():
					return watcher.Close()
				}
			}
		})
	}
	return eg.Wait()
}

func (s *composeService) refresh(ctx context.Context, project *types.Project, service string) error {
	err := s.build(ctx, project, api.BuildOptions{
		Services: []string{service},
	})
	if err != nil {
		return err
	}
	return s.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{
			Services: []string{service},
			Recreate: api.RecreateForce,
		},
	})
}
