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
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"gotest.tools/v3/assert"

	realcompose "github.com/docker/compose/v5/pkg/compose"
)

// unsupportedAttrFixture writes a compose file with an attribute
// (deploy.mode) that the unsupported-attribute check always flags, so any
// code path that loads it can be checked for whether it warns or stays
// silent.
func unsupportedAttrFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "compose.yaml")
	content := `
services:
  web:
    image: alpine
    deploy:
      mode: replicated
`
	assert.NilError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// captureWarnings runs fn with a global logrus hook installed and returns
// every message logged during the call.
func captureWarnings(t *testing.T, fn func()) []string {
	t.Helper()
	hook := logrustest.NewGlobal()
	logrus.SetOutput(io.Discard)
	defer func() {
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
		logrus.SetOutput(os.Stderr)
	}()
	fn()
	messages := make([]string, len(hook.Entries))
	for i, entry := range hook.Entries {
		messages[i] = entry.Message
	}
	return messages
}

// ToProject is the "real" load path used by commands that act on the full
// compose model (build, config, run, scale, watch, ...): it must warn.
func TestToProject_WarnsOnUnsupportedAttributes(t *testing.T) {
	opts := &ProjectOptions{ConfigPaths: []string{unsupportedAttrFixture(t)}}
	backend, err := realcompose.NewComposeService(nil)
	assert.NilError(t, err)

	messages := captureWarnings(t, func() {
		_, _, err := opts.ToProject(t.Context(), nil, backend, nil, warnUnsupportedAttributes)
		assert.NilError(t, err)
	})

	assert.Assert(t, len(messages) > 0, "ToProject should warn about deploy.mode")
}

func TestToProject_SkipsWarningWhenRequested(t *testing.T) {
	opts := &ProjectOptions{ConfigPaths: []string{unsupportedAttrFixture(t)}}
	backend, err := realcompose.NewComposeService(nil)
	assert.NilError(t, err)

	messages := captureWarnings(t, func() {
		_, _, err := opts.ToProject(t.Context(), nil, backend, nil, skipUnsupportedAttributesWarning)
		assert.NilError(t, err)
	})

	assert.Equal(t, len(messages), 0)
}

// projectOrName and toProjectName are lightweight project-name resolution
// helpers used by commands that operate on already-running
// containers/services by name (down, stop, ps, logs, ...) or by shell
// completion (completeServiceNames, completeProfileNames). Warning here
// would fire on every such invocation regardless of whether the command has
// anything to do with the flagged attribute, and even during tab-completion
// — see docker/compose#14196 review discussion.
func TestProjectOrName_DoesNotWarnOnUnsupportedAttributes(t *testing.T) {
	opts := &ProjectOptions{ConfigPaths: []string{unsupportedAttrFixture(t)}}

	messages := captureWarnings(t, func() {
		_, _, err := opts.projectOrName(t.Context(), nil)
		assert.NilError(t, err)
	})

	assert.Equal(t, len(messages), 0)
}

func TestToProjectName_DoesNotWarnOnUnsupportedAttributes(t *testing.T) {
	opts := &ProjectOptions{ConfigPaths: []string{unsupportedAttrFixture(t)}}

	messages := captureWarnings(t, func() {
		_, err := opts.toProjectName(t.Context(), nil)
		assert.NilError(t, err)
	})

	assert.Equal(t, len(messages), 0)
}
