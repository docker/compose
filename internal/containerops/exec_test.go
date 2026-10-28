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

package containerops

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestExecRejectsMissingOptions(t *testing.T) {
	tests := map[string]api.ExecOptionsProvider{
		"nil":             nil,
		"nil ExecOptions": (*api.ExecOptions)(nil),
		"nil RunOptions":  (*api.RunOptions)(nil),
	}
	for name, provider := range tests {
		t.Run(name, func(t *testing.T) {
			backend := &fakeBackend{}
			_, err := New(backend).Exec(t.Context(), "demo", provider)
			assert.ErrorIs(t, err, api.ErrInvalidOption)
			assert.Equal(t, backend.asked, "", "the backend must not be reached")
		})
	}
}
