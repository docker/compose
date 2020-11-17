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

func inDependencyOrder(ctx context.Context, project *types.Project, fn func(types.ServiceConfig) error) error {
	eg, ctx := errgroup.WithContext(ctx)
	var (
		scheduled []string
		ready []string
	)
	results := make(chan string)
	for len(ready) < len(project.Services) {
		for _, service := range project.Services {
			if contains(scheduled, service.Name) {
				continue
			}
			if containsAll(ready, service.GetDependencies()) {
				service := service
				scheduled = append(scheduled, service.Name)
				eg.Go(func() error {
					err := fn(service)
					if err != nil {
						close(results)
						return err
					}
					results <- service.Name
					return nil
				})
			}
		}
		result, ok := <-results
		if !ok {
			break
		}
		ready = append(ready, result)
	}
	return eg.Wait()
}
