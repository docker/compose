/*
   Copyright 2020 Docker, Inc.

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

package cmd

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"

	_ "github.com/docker/compose-cli/example"
	"github.com/docker/compose-cli/tests/framework"
)

func TestInspectId(t *testing.T) {
	c := framework.NewTestCLI(t)
	err := runInspect(c.Context(), "id")
	assert.NilError(t, err)
	golden.Assert(t, c.GetStdOut(), "inspect-out-id.golden")
}
