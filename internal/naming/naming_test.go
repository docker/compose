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

package naming

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"
)

func TestContainerName(t *testing.T) {
	assert.Equal(t, ContainerName("proj", types.ServiceConfig{Name: "web"}, 2), "proj-web-2")
	assert.Equal(t, ContainerName("proj", types.ServiceConfig{Name: "web", ContainerName: "custom"}, 2), "custom")
}

func TestCanonicalContainerName(t *testing.T) {
	assert.Equal(t, CanonicalContainerName(container.Summary{Names: []string{"/proj-web-1", "/other/web"}}), "proj-web-1")
	assert.Equal(t, CanonicalContainerName(container.Summary{Names: []string{"/linked_by/web"}}), "linked_by/web")
	assert.Equal(t, CanonicalContainerName(container.Summary{ID: "0123456789abcdef"}), "0123456789ab")
}
