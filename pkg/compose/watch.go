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

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type DevelopmentConfig struct {
}

func (s *composeService) Watch(ctx context.Context, project *types.Project, services []string, options api.WatchOptions) error {
	fmt.Fprintln(s.stderr(), "not implemented yet")

	eg, ctx := errgroup.WithContext(ctx)
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

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		fmt.Println("watching " + context)
		err = watcher.Add(context)
		if err != nil {
			return err
		}
		eg.Go(func() error {
			defer watcher.Close() //nolint:errcheck
			for {
				select {
				case <-ctx.Done():
					return nil
				case event := <-watcher.Events:
					log.Println("fs event :", event.String())
				case err := <-watcher.Errors:
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
