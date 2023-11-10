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
	"os"
	"path/filepath"
	"strings"
	"fmt"
	"testing"

	"github.com/docker/compose/v2/pkg/mocks"
	"github.com/docker/cli/opts"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/cli/cli/streams"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRunList_quietAndFilterFlags(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	dir := t.TempDir()
	out := filepath.Join(dir, "output.txt")
	f, err := os.Create(out)
	assert.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()
	stdout := streams.NewOut(f)

	dockerCliMock := mocks.NewMockCli(mockCtrl)
	dockerCliMock.EXPECT().
		Out().
		Return(stdout).
		Times(2)
	backendMock := mocks.NewMockService(mockCtrl)
	backendMock.EXPECT().
		List(gomock.Eq(ctx), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ api.ListOptions) ([]api.Stack, error) {
			return []api.Stack{
				{ Name: "test1" },
				{ Name: "test2" },
				{ Name: "shouldSkip" },
				{ Name: "anotherSkip" },
			}, nil
		}).
		Times(1)

	filter := opts.NewFilterOpt()
	filter.Set("name=^test*.$")

	options := lsOptions{
		Format: "table",
		Quiet: true,
		All: false,
		Filter: filter,
	}

	err = runList(ctx, dockerCliMock, backendMock, options)
	assert.NoError(t, err)

	output, err := os.ReadFile(out)
	assert.NoError(t, err)

	fmt.Println(string(output))
	names := strings.Split(string(output), "\n")

	assert.Equal(t, 3, len(names))
	assert.Equal(t, "test1", names[0])
	assert.Equal(t, "test2", names[1])
	assert.Equal(t, "", names[2])
}
