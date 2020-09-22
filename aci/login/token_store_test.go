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

package login

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestCreateStoreFromExistingFolder(t *testing.T) {
	existingDir, err := ioutil.TempDir("", "test_store")
	assert.NilError(t, err)

	storePath := filepath.Join(existingDir, tokenStoreFilename)
	store, err := newTokenStore(storePath)
	assert.NilError(t, err)
	assert.Equal(t, store.filePath, storePath)
}

func TestCreateStoreFromNonExistingFolder(t *testing.T) {
	existingDir, err := ioutil.TempDir("", "test_store")
	assert.NilError(t, err)

	storePath := filepath.Join(existingDir, "new", tokenStoreFilename)
	store, err := newTokenStore(storePath)
	assert.NilError(t, err)
	assert.Equal(t, store.filePath, storePath)

	newDir, err := os.Stat(filepath.Join(existingDir, "new"))
	assert.NilError(t, err)
	assert.Assert(t, newDir.Mode().IsDir())
}

func TestErrorIfParentFolderIsAFile(t *testing.T) {
	existingDir, err := ioutil.TempFile("", "test_store")
	assert.NilError(t, err)

	storePath := filepath.Join(existingDir.Name(), tokenStoreFilename)
	_, err = newTokenStore(storePath)
	assert.Error(t, err, "cannot use path "+storePath+" ; "+existingDir.Name()+" already exists and is not a directory")
}
