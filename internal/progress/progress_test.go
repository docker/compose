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

package progress

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestEvents(t *testing.T) {
	assert.DeepEqual(t, Creating("web"), api.Resource{ID: "web", Status: api.Working, Text: api.StatusCreating})
	assert.DeepEqual(t, Created("web"), api.Resource{ID: "web", Status: api.Done, Text: api.StatusCreated})
	assert.DeepEqual(t, Removing("web"), api.Resource{ID: "web", Status: api.Working, Text: api.StatusRemoving})
	assert.DeepEqual(t, Removed("web"), api.Resource{ID: "web", Status: api.Done, Text: api.StatusRemoved})
	assert.DeepEqual(t, Error("web", "boom"), api.Resource{ID: "web", Status: api.Error, Text: api.StatusError, Details: "boom"})
	assert.DeepEqual(t, New("web", api.Warning, "Skipped", "why"), api.Resource{ID: "web", Status: api.Warning, Text: "Skipped", Details: "why"})
	assert.DeepEqual(t, New("web", api.Warning, "Skipped"), api.Resource{ID: "web", Status: api.Warning, Text: "Skipped"})
}
