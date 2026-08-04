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
	"bytes"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestProfilesOnlyServices(t *testing.T) {
	newProject := func(activeProfiles ...string) *types.Project {
		project := &types.Project{
			Name: "test",
			Services: types.Services{
				"core": types.ServiceConfig{Name: "core"},
			},
			DisabledServices: types.Services{
				"svc-a":  types.ServiceConfig{Name: "svc-a", Profiles: []string{"a"}},
				"svc-ab": types.ServiceConfig{Name: "svc-ab", Profiles: []string{"a", "b"}},
				"svc-b":  types.ServiceConfig{Name: "svc-b", Profiles: []string{"b"}},
			},
		}
		if len(activeProfiles) == 0 {
			// mimic the loader behavior when no profile is set: COMPOSE_PROFILES
			// being unset yields a single blank profile name
			project.Profiles = []string{""}
			return project
		}
		withProfiles, err := project.WithProfiles(activeProfiles)
		assert.NilError(t, err)
		return withProfiles
	}

	t.Run("rejects explicit service names", func(t *testing.T) {
		_, _, err := profilesOnlyServices(newProject("a"), []string{"svc-a"}, "Stopping", &bytes.Buffer{})
		assert.ErrorContains(t, err, "cannot be combined with service names")
	})

	t.Run("requires a project", func(t *testing.T) {
		_, _, err := profilesOnlyServices(nil, nil, "Stopping", &bytes.Buffer{})
		assert.ErrorContains(t, err, "requires the project's compose file(s)")
	})

	t.Run("restricts to services of the active profiles", func(t *testing.T) {
		out := &bytes.Buffer{}
		_, services, err := profilesOnlyServices(newProject("a"), nil, "Stopping", out)
		assert.NilError(t, err)
		assert.DeepEqual(t, services, []string{"svc-a", "svc-ab"})
		assert.Equal(t, out.String(), "Stopping services in profiles [a]\n")
	})

	t.Run("no active profile targets all profiled services", func(t *testing.T) {
		out := &bytes.Buffer{}
		project, services, err := profilesOnlyServices(newProject(), nil, "Stopping", out)
		assert.NilError(t, err)
		assert.DeepEqual(t, services, []string{"svc-a", "svc-ab", "svc-b"})
		assert.Equal(t, out.String(), "Stopping services from all profiles [a b] as none is active\n")
		// the returned project must have the targeted services enabled, as the
		// backend only acts on enabled services
		for _, name := range services {
			_, enabled := project.Services[name]
			assert.Assert(t, enabled, "service %s should be enabled in the returned project", name)
		}
	})

	t.Run("active profile matching no service is a no-op", func(t *testing.T) {
		out := &bytes.Buffer{}
		_, services, err := profilesOnlyServices(newProject("unknown"), nil, "Stopping", out)
		assert.NilError(t, err)
		assert.Equal(t, len(services), 0)
		assert.Equal(t, out.String(), "no services matched the active profiles [unknown]\n")
	})

	t.Run("project without profiles is a no-op", func(t *testing.T) {
		project := &types.Project{
			Name:     "test",
			Services: types.Services{"core": types.ServiceConfig{Name: "core"}},
			Profiles: []string{""},
		}
		out := &bytes.Buffer{}
		_, services, err := profilesOnlyServices(project, nil, "Stopping", out)
		assert.NilError(t, err)
		assert.Equal(t, len(services), 0)
		assert.Equal(t, out.String(), "no service in this project uses profiles\n")
	})
}
