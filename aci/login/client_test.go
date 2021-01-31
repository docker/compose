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

func TestClearErrorMessageIfNotAlreadyLoggedIn(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_store")
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	_, _, err = getClientSetupDataImpl(filepath.Join(dir, tokenStoreFilename))
	assert.ErrorContains(t, err, "not logged in to azure, you need to run \"docker login azure\" first")
}
