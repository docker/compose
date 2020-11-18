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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/compose-spec/compose-go/types"
)

func TestInDependencyOrder(t *testing.T) {
	order := make(chan string)
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name: "test1",
				DependsOn: map[string]types.ServiceDependency{
					"test2": {},
				},
			},
			{
				Name: "test2",
				DependsOn: map[string]types.ServiceDependency{
					"test3": {},
				},
			},
			{
				Name: "test3",
			},
		},
	}
	//nolint:errcheck, unparam
	go inDependencyOrder(context.TODO(), &project, func(ctx context.Context, config types.ServiceConfig) error {
		order <- config.Name
		return nil
	})
	assert.Equal(t, <-order, "test3")
	assert.Equal(t, <-order, "test2")
	assert.Equal(t, <-order, "test1")
}
