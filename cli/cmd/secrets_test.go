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

package cmd

import (
	"bytes"
	"testing"

	"gotest.tools/assert"
	"gotest.tools/v3/golden"

	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/formatter"
)

func TestPrintList(t *testing.T) {
	secretList := []secrets.Secret{
		{
			ID:          "123",
			Name:        "secret123",
			Description: "secret 1,2,3",
		},
	}
	out := &bytes.Buffer{}
	assert.NilError(t, printSecretList(formatter.PRETTY, out, secretList))
	golden.Assert(t, out.String(), "secrets-out.golden")

	out.Reset()
	assert.NilError(t, printSecretList(formatter.JSON, out, secretList))
	golden.Assert(t, out.String(), "secrets-out-json.golden")
}
