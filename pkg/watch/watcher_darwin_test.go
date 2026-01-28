//go:build fsnotify

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

package watch

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestFseventNotifyCloseIdempotent(t *testing.T) {
	// Create a watcher with a temporary directory
	tmpDir := t.TempDir()
	watcher, err := newWatcher([]string{tmpDir})
	assert.NilError(t, err)

	// Start the watcher
	err = watcher.Start()
	assert.NilError(t, err)

	// Close should work the first time
	err = watcher.Close()
	assert.NilError(t, err)

	// Close should be idempotent - calling it again should not panic
	err = watcher.Close()
	assert.NilError(t, err)

	// Even a third time should be safe
	err = watcher.Close()
	assert.NilError(t, err)
}
