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
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

func TestProviderMetadata_IsEmpty(t *testing.T) {
	param := []ParameterMetadata{{Name: "x"}}

	tests := []struct {
		name     string
		metadata ProviderMetadata
		want     bool
	}{
		{
			name:     "empty metadata",
			metadata: ProviderMetadata{},
			want:     true,
		},
		{
			name:     "only Description set",
			metadata: ProviderMetadata{Description: "something"},
			want:     false,
		},
		{
			name:     "only Up.Parameters set",
			metadata: ProviderMetadata{Up: CommandMetadata{Parameters: param}},
			want:     false,
		},
		{
			name:     "only Down.Parameters set",
			metadata: ProviderMetadata{Down: CommandMetadata{Parameters: param}},
			want:     false,
		},
		{
			name:     "only Stop set is empty",
			metadata: ProviderMetadata{Stop: &CommandMetadata{}},
			want:     true,
		},
		{
			name: "all fields set",
			metadata: ProviderMetadata{
				Description: "full",
				Up:          CommandMetadata{Parameters: param},
				Down:        CommandMetadata{Parameters: param},
				Stop:        &CommandMetadata{Parameters: param},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.metadata.IsEmpty(), tc.want)
		})
	}
}

func TestProviderMetadata_JSONUnmarshal(t *testing.T) {
	raw := `{"description":"x","up":{"parameters":[{"name":"a"}]},"down":{"parameters":[{"name":"b"}]},"stop":{"parameters":[{"name":"c"}]}}`

	var metadata ProviderMetadata
	err := json.Unmarshal([]byte(raw), &metadata)
	assert.NilError(t, err)
	assert.Equal(t, metadata.Description, "x")
	assert.Equal(t, metadata.Up.Parameters[0].Name, "a")
	assert.Equal(t, metadata.Down.Parameters[0].Name, "b")
	assert.Assert(t, metadata.Stop != nil, "Stop should be non-nil when present in JSON")
	assert.Equal(t, metadata.Stop.Parameters[0].Name, "c")
}

func TestProviderMetadata_StopAbsent(t *testing.T) {
	raw := `{"description":"x","up":{"parameters":[]},"down":{"parameters":[]}}`

	var metadata ProviderMetadata
	err := json.Unmarshal([]byte(raw), &metadata)
	assert.NilError(t, err)
	assert.Assert(t, metadata.Stop == nil, "Stop should be nil when absent from JSON")
}

func TestProviderMetadata_StopAdvertisedWithoutParameters(t *testing.T) {
	raw := `{"stop":{"parameters":null}}`

	var metadata ProviderMetadata
	err := json.Unmarshal([]byte(raw), &metadata)
	assert.NilError(t, err)
	assert.Assert(t, metadata.Stop != nil, "Stop should be non-nil when key present even with null parameters")
}
