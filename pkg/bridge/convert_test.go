/*
   Copyright 2026 Docker Compose CLI authors

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

package bridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func TestLoadAdditionalResources_BuildOnlySkipsPull(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	dockerCLI := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	dockerCLI.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().ImageInspect(gomock.Any(), "test-api").
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"api": {
				Name:   "api",
				Build:  &types.BuildConfig{Context: "."},
				Expose: []string{"8080"},
			},
		},
	}

	actual, err := LoadAdditionalResources(t.Context(), dockerCLI, project)
	assert.NilError(t, err)
	assert.Equal(t, actual.Services["api"].Image, "test-api")
	assert.DeepEqual(t, actual.Services["api"].Expose, types.StringOrNumberList{"8080"})
}

func TestIsEmptyOrMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist-yet")
	empty, err := isEmptyOrMissingDir(missing)
	assert.NilError(t, err)
	assert.Equal(t, empty, true)

	emptyDir := t.TempDir()
	empty, err = isEmptyOrMissingDir(emptyDir)
	assert.NilError(t, err)
	assert.Equal(t, empty, true)

	nonEmptyDir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(nonEmptyDir, "README.md"), []byte("do not delete me"), 0o600))
	empty, err = isEmptyOrMissingDir(nonEmptyDir)
	assert.NilError(t, err)
	assert.Equal(t, empty, false)
}

func failIfCalled(t *testing.T) Confirm {
	t.Helper()
	return func(string, bool) (bool, error) {
		t.Fatal("Confirm should not be called")
		return false, nil
	}
}

func TestPrepareOutputDir_EmptyOrMissingDirNeedsNoConfirmation(t *testing.T) {
	for name, dir := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "does-not-exist-yet"),
		"empty":   t.TempDir(),
	} {
		t.Run(name, func(t *testing.T) {
			assert.NilError(t, prepareOutputDir(ConvertOptions{Output: dir, Confirm: failIfCalled(t)}))

			info, err := os.Stat(dir)
			assert.NilError(t, err)
			assert.Assert(t, info.IsDir())
		})
	}
}

func TestPrepareOutputDir_NilConfirmRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("do not delete me"), 0o600))

	err := prepareOutputDir(ConvertOptions{Output: dir})
	assert.ErrorContains(t, err, "not confirmed")

	_, statErr := os.Stat(filepath.Join(dir, "README.md"))
	assert.NilError(t, statErr, "user file must not be deleted")
}

func TestPrepareOutputDir_ConfirmedWipesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "stale.yaml"), []byte("stale"), 0o600))
	confirm := func(string, bool) (bool, error) { return true, nil }

	assert.NilError(t, prepareOutputDir(ConvertOptions{Output: dir, Confirm: confirm}))

	_, err := os.Stat(filepath.Join(dir, "stale.yaml"))
	assert.Assert(t, os.IsNotExist(err))
}

func TestPrepareOutputDir_DeclinedPreservesFiles(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("do not delete me"), 0o600))
	confirm := func(string, bool) (bool, error) { return false, nil }

	err := prepareOutputDir(ConvertOptions{Output: dir, Confirm: confirm})
	assert.ErrorContains(t, err, "not confirmed")

	_, statErr := os.Stat(filepath.Join(dir, "README.md"))
	assert.NilError(t, statErr, "user file must not be deleted")
}
