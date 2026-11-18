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
	"testing"

	logrustest "github.com/sirupsen/logrus/hooks/test"
	"gotest.tools/v3/assert"
)

func TestNormalizeYesFlag(t *testing.T) {
	hook := logrustest.NewGlobal()
	assert.Equal(t, string(normalizeYesFlag(nil, "yes")), "yes")
	assert.Equal(t, len(hook.AllEntries()), 0)

	assert.Equal(t, string(normalizeYesFlag(nil, "y")), "yes")
	assert.Equal(t, len(hook.AllEntries()), 1)
	assert.Equal(t, hook.LastEntry().Message, "--y is deprecated, please use --yes instead")
}
