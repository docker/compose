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
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/require"
)

var project = types.Project{
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

func TestInDependencyUpCommandOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var order []string
	err := InDependencyOrder(ctx, &project, func(ctx context.Context, service string) error {
		order = append(order, service)
		return nil
	})
	require.NoError(t, err, "Error during iteration")
	require.Equal(t, []string{"test3", "test2", "test1"}, order)
}

func TestInDependencyReverseDownCommandOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var order []string
	err := InReverseDependencyOrder(ctx, &project, func(ctx context.Context, service string) error {
		order = append(order, service)
		return nil
	})
	require.NoError(t, err, "Error during iteration")
	require.Equal(t, []string{"test1", "test2", "test3"}, order)
}
