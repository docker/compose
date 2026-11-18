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

	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestFilters(t *testing.T) {
	assert.Equal(t, LabelFilter("k", "v"), "k=v")
	assert.Equal(t, ServiceFilter("web"), api.ServiceLabel+"=web")
	assert.DeepEqual(t, ProjectFilter("proj")["label"], map[string]bool{api.ProjectLabel + "=proj": true})
}

func TestInServices(t *testing.T) {
	web := container.Summary{ID: "1", Labels: map[string]string{api.ServiceLabel: "web"}}
	db := container.Summary{ID: "2", Labels: map[string]string{api.ServiceLabel: "db"}}
	assert.DeepEqual(t, InServices([]container.Summary{web, db}, []string{"db"}), []container.Summary{db})
	assert.Equal(t, len(InServices([]container.Summary{web, db}, []string{"other"})), 0)
}
