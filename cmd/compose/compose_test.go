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

func TestFilterServices(t *testing.T) {
	p := &types.Project{
		Services: types.Services{
			"foo": {
				Name:  "foo",
				Links: []string{"bar"},
			},
			"bar": {
				Name: "bar", WorkloadSpec: types.WorkloadSpec{DependsOn: map[string]types.ServiceDependency{
					"zot": {},
				}},
			},
			"zot": {
				Name: "zot",
			},
			"qix": {
				Name: "qix",
			},
		},
	}
	p, err := p.WithSelectedServices([]string{"bar"})
	assert.NilError(t, err)

	assert.Equal(t, len(p.Services), 2)
	_, err = p.GetService("bar")
	assert.NilError(t, err)
	_, err = p.GetService("zot")
	assert.NilError(t, err)
}

// projectOrName backs every service-targeting command except run/create
// (down, stop, kill, pause, unpause, logs, rm, ps, events, start, ...): a
// job target must be refused the same way regardless of which of them is
// used, and the refusal must not be masked by the COMPOSE_PROJECT_NAME
// fallback below it.
func TestProjectOrNameRefusesJob(t *testing.T) {
	opts := jobTargetErrFixture(t)

	t.Run("a job target is refused with a clear error", func(t *testing.T) {
		_, _, err := opts.projectOrName(t.Context(), nil, "migrate")
		assert.Error(t, err, `job "migrate" can only be triggered with "docker compose run"`)
	})

	t.Run("COMPOSE_PROJECT_NAME must not mask the refusal behind a silent fallback", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "test")
		_, _, err := opts.projectOrName(t.Context(), nil, "migrate")
		assert.Error(t, err, `job "migrate" can only be triggered with "docker compose run"`)
	})

	t.Run("a real typo among several targets keeps its own error, not a same-invocation job's", func(t *testing.T) {
		_, _, err := opts.projectOrName(t.Context(), nil, "typo", "migrate")
		assert.ErrorContains(t, err, "no such service: typo")
	})
}
