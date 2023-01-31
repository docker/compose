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
	"log"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/compose/v2/pkg/watch"
	"github.com/jonboulle/clockwork"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type DevelopmentConfig struct {
}

const quietPeriod = 2 * time.Second

func (s *composeService) Watch(ctx context.Context, project *types.Project, services []string, options api.WatchOptions) error {
	fmt.Fprintln(s.stderr(), "not implemented yet")

	eg, ctx := errgroup.WithContext(ctx)
	needRefresh := make(chan string)
	eg.Go(func() error {
		clock := clockwork.NewRealClock()
		debounce(ctx, clock, quietPeriod, needRefresh, func(services []string) {
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
		})
		return nil
	})

	err := project.WithServices(services, func(service types.ServiceConfig) error {
		var config DevelopmentConfig
		if y, ok := service.Extensions["x-develop"]; ok {
			err := mapstructure.Decode(y, &config)
			if err != nil {
				return err
			}
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

		fmt.Println("watching " + context)
		err = watcher.Start()
		if err != nil {
			return err
		}

		eg.Go(func() error {
			defer watcher.Close() //nolint:errcheck
			for {
				select {
				case <-ctx.Done():
					return nil
				case event := <-watcher.Events():
					log.Println("fs event :", event.Path())
					needRefresh <- service.Name
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
