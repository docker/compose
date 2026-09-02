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

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/v2/cli"
	"gotest.tools/v3/assert"
)

func TestWithEnvFilesSkipsUnreadableDefault(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	assert.NilError(t, os.Mkdir(blocked, 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(blocked, ".env"), []byte("FOO=1\n"), 0o600))
	assert.NilError(t, os.Chmod(blocked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })

	if _, err := os.Stat(filepath.Join(blocked, ".env")); err == nil {
		t.Skip("process can still stat the file (running as root?)")
	}

	opts, err := cli.NewProjectOptions(nil,
		cli.WithWorkingDirectory(blocked),
		WithEnvFiles(),
	)
	assert.NilError(t, err)
	assert.Equal(t, len(opts.EnvFiles), 0)
}

func TestWithEnvFilesKeepsExplicitPath(t *testing.T) {
	opts, err := cli.NewProjectOptions(nil, WithEnvFiles("/no/such/.env"))
	assert.NilError(t, err)
	assert.DeepEqual(t, opts.EnvFiles, []string{"/no/such/.env"})
}

func TestWithEnvFilesLoadsReadableDefault(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=1\n"), 0o600))
	opts, err := cli.NewProjectOptions(nil,
		cli.WithWorkingDirectory(dir),
		WithEnvFiles(),
	)
	assert.NilError(t, err)
	assert.Equal(t, len(opts.EnvFiles), 1)
}
