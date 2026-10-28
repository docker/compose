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

// Options that cannot be honored are rejected before the engine is reached:
// the service has no client, so any attempt to go further would panic.
func TestOperationsRejectInvalidOptions(t *testing.T) {
	s := &composeService{events: &ignore{}}

	err := s.Logs(t.Context(), "test", nil, api.LogOptions{Index: 1})
	assert.Assert(t, errors.Is(err, api.ErrIncompatibleOptions), "logs: got %v", err)

	err = s.Events(t.Context(), "test", api.EventsOptions{})
	assert.Assert(t, errors.Is(err, api.ErrInvalidOption), "events: got %v", err)

	err = s.Publish(t.Context(), &types.Project{Name: "test"}, "example.com/test", api.PublishOptions{OCIVersion: "1.2"})
	assert.Assert(t, errors.Is(err, api.ErrInvalidOption), "publish: got %v", err)
}
