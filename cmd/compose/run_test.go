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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestIsJobName_Service(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"web": {Name: "web"},
		},
		Jobs: types.Jobs{
			"migrate": {Name: "migrate"},
		},
	}
	assert.Assert(t, !isJobName(project, "web"))
}

func TestIsJobName_Job(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"web": {Name: "web"},
		},
		Jobs: types.Jobs{
			"migrate": {Name: "migrate"},
		},
	}
	assert.Assert(t, isJobName(project, "migrate"))
}

func TestIsJobName_Neither(t *testing.T) {
	project := &types.Project{
		Services: types.Services{},
		Jobs:     types.Jobs{},
	}
	assert.Assert(t, !isJobName(project, "nonexistent"))
}

func TestIsJobName_ServiceTakesPrecedence(t *testing.T) {
	// If a name exists as both service and job, service wins
	project := &types.Project{
		Services: types.Services{
			"ambiguous": {Name: "ambiguous"},
		},
		Jobs: types.Jobs{
			"ambiguous": {Name: "ambiguous"},
		},
	}
	assert.Assert(t, !isJobName(project, "ambiguous"))
}
