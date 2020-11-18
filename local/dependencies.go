// +build local

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

package local

import (
	"context"

	"github.com/compose-spec/compose-go/types"
	"golang.org/x/sync/errgroup"
)

func inDependencyOrder(ctx context.Context, project *types.Project, fn func(context.Context, types.ServiceConfig) error) error {
	var (
		scheduled []string
		ready     []string
	)
	services := sortByDependency(project.Services)

	results := make(chan string)
	errors := make(chan error)
	eg, ctx := errgroup.WithContext(ctx)
	for len(ready) < len(services) {
		for _, service := range services {
			if contains(scheduled, service.Name) {
				continue
			}
			if containsAll(ready, service.GetDependencies()) {
				service := service
				scheduled = append(scheduled, service.Name)
				eg.Go(func() error {
					err := fn(ctx, service)
					if err != nil {
						errors <- err
						return err
					}
					results <- service.Name
					return nil
				})
			}
		}
		select {
		case result := <-results:
			ready = append(ready, result)
		case err := <-errors:
			return err
		}
	}
	return eg.Wait()
}

// sortByDependency sort a Service slice so it can be processed in respect to dependency ordering
func sortByDependency(services types.Services) types.Services {
	var sorted types.Services
	var done []string
	for len(sorted) < len(services) {
		for _, s := range services {
			if contains(done, s.Name) {
				continue
			}
			if containsAll(done, s.GetDependencies()) {
				sorted = append(sorted, s)
				done = append(done, s.Name)
			}
		}
	}
	return sorted
}
