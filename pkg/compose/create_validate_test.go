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
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// An unknown recreate policy must be rejected before the engine is reached: the
// service has no client, so any attempt to go further would panic.
func TestCreateAndUpRejectInvalidOptions(t *testing.T) {
	s := &composeService{events: &ignore{}}
	project := &types.Project{Name: "test"}
	invalid := api.CreateOptions{Recreate: "forced"}

	err := s.Create(t.Context(), project, invalid)
	assert.Assert(t, errors.Is(err, api.ErrInvalidOption), "got %v", err)

	err = s.Up(t.Context(), project, api.UpOptions{Create: invalid})
	assert.Assert(t, errors.Is(err, api.ErrInvalidOption), "got %v", err)
}
