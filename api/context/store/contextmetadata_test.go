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

package store

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestDockerContextMetadataKeepAdditionalFields(t *testing.T) {
	c := ContextMetadata{
		Description:       "test",
		Type:              "aci",
		StackOrchestrator: "swarm",
		AdditionalFields: map[string]interface{}{
			"foo": "bar",
		},
	}
	jsonBytes, err := json.Marshal(c)
	assert.NilError(t, err)
	assert.Equal(t, string(jsonBytes), `{"Description":"test","StackOrchestrator":"swarm","Type":"aci","foo":"bar"}`)

	var c2 ContextMetadata
	err = json.Unmarshal(jsonBytes, &c2)
	assert.NilError(t, err)
	assert.Equal(t, c2.AdditionalFields["foo"], "bar")
	assert.Equal(t, c2.Type, "aci")
	assert.Equal(t, c2.StackOrchestrator, "swarm")
	assert.Equal(t, c2.Description, "test")
}
