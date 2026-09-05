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
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

// https://github.com/docker/compose/issues/11867
// driver: copy must read a `file`-based secret or config from the CLIENT's
// local filesystem, so it can be delivered to a container on a remote
// Docker host that has no access to that path.
func TestResolveFileContent_CopyDriver(t *testing.T) {
	s := &composeService{}
	path := filepath.Join(t.TempDir(), "secret.txt")
	assert.NilError(t, os.WriteFile(path, []byte("hunter2"), 0o600))

	content, err := s.resolveFileContent(&types.Project{}, types.FileObjectConfig{
		Name:   "db_password",
		Driver: copyDriver,
		File:   path,
	}, secretMount)
	assert.NilError(t, err)
	assert.Equal(t, content, "hunter2")
}

func TestResolveFileContent_CopyDriverMissingFile(t *testing.T) {
	s := &composeService{}
	_, err := s.resolveFileContent(&types.Project{}, types.FileObjectConfig{
		Name:   "db_password",
		Driver: copyDriver,
		File:   filepath.Join(t.TempDir(), "missing.txt"),
	}, secretMount)
	assert.ErrorContains(t, err, "reading secret")
}

// Content and Environment still take priority over driver: copy, matching
// resolveFileContent's existing precedence.
func TestResolveFileContent_ContentTakesPriorityOverCopyDriver(t *testing.T) {
	s := &composeService{}
	content, err := s.resolveFileContent(&types.Project{}, types.FileObjectConfig{
		Name:    "db_password",
		Content: "inline-value",
		Driver:  copyDriver,
		File:    filepath.Join(t.TempDir(), "missing.txt"),
	}, secretMount)
	assert.NilError(t, err)
	assert.Equal(t, content, "inline-value")
}

// Without driver: copy, a File-only source resolves to no content: it stays
// exclusively the bind-mount path's responsibility (buildContainerSecretMounts).
func TestResolveFileContent_PlainFileIsNotCopied(t *testing.T) {
	s := &composeService{}
	content, err := s.resolveFileContent(&types.Project{}, types.FileObjectConfig{
		Name: "db_password",
		File: filepath.Join(t.TempDir(), "secret.txt"),
	}, secretMount)
	assert.NilError(t, err)
	assert.Equal(t, content, "")
}
