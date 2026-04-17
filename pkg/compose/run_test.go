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

	"github.com/docker/compose/v5/pkg/api"
)

func TestResolveRunTarget_Service(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"web": {
				Name: "web",
				ContainerSpec: types.ContainerSpec{
					Image: "nginx",
				},
			},
		},
	}
	svc, job, err := resolveRunTarget(project, api.RunOptions{Service: "web"})
	assert.NilError(t, err)
	assert.Assert(t, svc != nil)
	assert.Assert(t, job == nil)
	assert.Equal(t, svc.Name, "web")
	assert.Equal(t, svc.Image, "nginx")
}

func TestResolveRunTarget_Job(t *testing.T) {
	project := &types.Project{
		Services: types.Services{},
		Jobs: types.Jobs{
			"migrate": {
				Name: "migrate",
				ContainerSpec: types.ContainerSpec{
					Image:   "myapp",
					Command: types.ShellCommand{"python", "manage.py", "migrate"},
					DependsOn: types.DependsOnConfig{
						"db": {Condition: "service_healthy"},
					},
				},
			},
		},
	}
	svc, job, err := resolveRunTarget(project, api.RunOptions{Job: "migrate"})
	assert.NilError(t, err)
	assert.Assert(t, svc == nil)
	assert.Assert(t, job != nil)
	assert.Equal(t, job.Name, "migrate")
	assert.Equal(t, job.Image, "myapp")
	assert.DeepEqual(t, []string(job.Command), []string{"python", "manage.py", "migrate"})
	assert.Equal(t, len(job.DependsOn), 1)
}

func TestResolveRunTarget_JobNotFound(t *testing.T) {
	project := &types.Project{
		Services: types.Services{},
		Jobs:     types.Jobs{},
	}
	_, _, err := resolveRunTarget(project, api.RunOptions{Job: "nonexistent"})
	assert.ErrorContains(t, err, "no such job: nonexistent")
}

func TestResolveRunTarget_ServiceNotFound(t *testing.T) {
	project := &types.Project{
		Services: types.Services{},
	}
	_, _, err := resolveRunTarget(project, api.RunOptions{Service: "nonexistent"})
	assert.ErrorContains(t, err, "nonexistent")
}

func TestResolveRunTarget_JobPreservesContainerSpec(t *testing.T) {
	envVal := "db"
	project := &types.Project{
		Services: types.Services{},
		Jobs: types.Jobs{
			"backup": {
				Name: "backup",
				ContainerSpec: types.ContainerSpec{
					Image:      "postgres",
					Command:    types.ShellCommand{"pg_dump"},
					WorkingDir: "/data",
					Environment: types.MappingWithEquals{
						"PGHOST": &envVal,
					},
					Volumes: []types.ServiceVolumeConfig{
						{
							Type:   types.VolumeTypeBind,
							Source: "/backups",
							Target: "/output",
						},
					},
				},
				Triggers: &types.TriggerConfig{
					Schedule: "0 2 * * *",
				},
			},
		},
	}
	_, job, err := resolveRunTarget(project, api.RunOptions{Job: "backup"})
	assert.NilError(t, err)
	assert.Equal(t, job.Image, "postgres")
	assert.Equal(t, job.WorkingDir, "/data")
	assert.Equal(t, *job.Environment["PGHOST"], "db")
	assert.Equal(t, len(job.Volumes), 1)
	assert.Equal(t, job.Volumes[0].Source, "/backups")
}
