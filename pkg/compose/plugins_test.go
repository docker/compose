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
	"os/exec"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
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

func TestExecutePlugin_ParsesSecretMessages(t *testing.T) {
	script := `printf '%s\n' ` +
		`'{"type":"setsecret","message":"db_password=hunter2"}' ` +
		`'{"type":"rawsetsecret","message":"api_key=s3cr3t=withpadding"}'`
	cmd := exec.Command("sh", "-c", script)

	s := &composeService{events: &ignore{}}
	variables, err := s.executePlugin(cmd, "up", types.ServiceConfig{Name: "provider"})
	assert.NilError(t, err)
	assert.Equal(t, variables.secrets["db_password"], "hunter2")
	assert.Equal(t, variables.rawSecrets["api_key"], "s3cr3t=withpadding")
}

func TestExecutePlugin_InvalidSecretMessage(t *testing.T) {
	cmd := exec.Command("sh", "-c", `printf '%s\n' '{"type":"setsecret","message":"no-equals-sign"}'`)

	s := &composeService{events: &ignore{}}
	_, err := s.executePlugin(cmd, "up", types.ServiceConfig{Name: "provider"})
	assert.ErrorContains(t, err, "invalid response from plugin")
}

func TestUpsertProviderSecret(t *testing.T) {
	project := &types.Project{Secrets: types.Secrets{}}

	secrets := upsertProviderSecret(project, nil, "database_db_password", "hunter2")
	assert.Equal(t, len(secrets), 1)
	assert.Equal(t, secrets[0].Source, "database_db_password")
	assert.Equal(t, project.Secrets["database_db_password"].Content, "hunter2")

	// Re-running the provider (idempotent `up`) must update the content
	// without appending a duplicate reference to the dependent service.
	secrets = upsertProviderSecret(project, secrets, "database_db_password", "rotated")
	assert.Equal(t, len(secrets), 1)
	assert.Equal(t, project.Secrets["database_db_password"].Content, "rotated")
}
