/*
   Copyright 2023 Docker Compose CLI authors

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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/davecgh/go-spew/spew"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunCreate(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	backend := mocks.NewMockService(ctrl)
	backend.EXPECT().Create(
		gomock.Eq(ctx),
		pullPolicy(""),
		deepEqual(defaultCreateOptions(true)),
	)

	createOpts := createOptions{}
	buildOpts := buildOptions{}
	project := sampleProject()
	err := runCreate(ctx, nil, backend, createOpts, buildOpts, project, nil)
	require.NoError(t, err)
}

func TestRunCreate_Build(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	backend := mocks.NewMockService(ctrl)
	backend.EXPECT().Create(
		gomock.Eq(ctx),
		pullPolicy("build"),
		deepEqual(defaultCreateOptions(true)),
	)

	createOpts := createOptions{
		Build: true,
	}
	buildOpts := buildOptions{}
	project := sampleProject()
	err := runCreate(ctx, nil, backend, createOpts, buildOpts, project, nil)
	require.NoError(t, err)
}

func TestRunCreate_NoBuild(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	backend := mocks.NewMockService(ctrl)
	backend.EXPECT().Create(
		gomock.Eq(ctx),
		pullPolicy(""),
		deepEqual(defaultCreateOptions(false)),
	)

	createOpts := createOptions{
		noBuild: true,
	}
	buildOpts := buildOptions{}
	project := sampleProject()
	err := runCreate(ctx, nil, backend, createOpts, buildOpts, project, nil)
	require.NoError(t, err)
}

func sampleProject() *types.Project {
	return &types.Project{
		Name: "test",
		Services: types.Services{
			"svc": {
				Name: "svc",
				Build: &types.BuildConfig{
					Context: ".",
				},
			},
		},
	}
}

func defaultCreateOptions(includeBuild bool) api.CreateOptions {
	var build *api.BuildOptions
	if includeBuild {
		bo := defaultBuildOptions()
		build = &bo
	}
	return api.CreateOptions{
		Build:                build,
		Services:             nil,
		RemoveOrphans:        false,
		IgnoreOrphans:        false,
		Recreate:             "diverged",
		RecreateDependencies: "diverged",
		Inherit:              true,
		Timeout:              nil,
		QuietPull:            false,
	}
}

func defaultBuildOptions() api.BuildOptions {
	return api.BuildOptions{
		Args:     make(types.MappingWithEquals),
		Progress: "auto",
	}
}

// deepEqual returns a nice diff on failure vs gomock.Eq when used
// on structs.
func deepEqual(x interface{}) gomock.Matcher {
	return gomock.GotFormatterAdapter(
		gomock.GotFormatterFunc(func(got interface{}) string {
			return cmp.Diff(x, got)
		}),
		gomock.Eq(x),
	)
}

func spewAdapter(m gomock.Matcher) gomock.Matcher {
	return gomock.GotFormatterAdapter(
		gomock.GotFormatterFunc(func(got interface{}) string {
			return spew.Sdump(got)
		}),
		m,
	)
}

type withPullPolicy struct {
	policy string
}

func pullPolicy(policy string) gomock.Matcher {
	return spewAdapter(withPullPolicy{policy: policy})
}

func (w withPullPolicy) Matches(x interface{}) bool {
	proj, ok := x.(*types.Project)
	if !ok || proj == nil || len(proj.Services) == 0 {
		return false
	}

	for _, svc := range proj.Services {
		if svc.PullPolicy != w.policy {
			return false
		}
	}

	return true
}

func (w withPullPolicy) String() string {
	return fmt.Sprintf("has pull policy %q for all services", w.policy)
}
