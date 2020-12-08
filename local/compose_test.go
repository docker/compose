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

package local

import (
	"os"
	"path/filepath"
	"testing"

	composetypes "github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types"
	mountTypes "github.com/docker/docker/api/types/mount"
	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/api/compose"
)

func TestContainersToStacks(t *testing.T) {
	containers := []types.Container{
		{
			ID:     "service1",
			State:  "running",
			Labels: map[string]string{projectLabel: "project1"},
		},
		{
			ID:     "service2",
			State:  "running",
			Labels: map[string]string{projectLabel: "project1"},
		},
		{
			ID:     "service3",
			State:  "running",
			Labels: map[string]string{projectLabel: "project2"},
		},
	}
	stacks, err := containersToStacks(containers)
	assert.NilError(t, err)
	assert.DeepEqual(t, stacks, []compose.Stack{
		{
			ID:     "project1",
			Name:   "project1",
			Status: "running(2)",
		},
		{
			ID:     "project2",
			Name:   "project2",
			Status: "running(1)",
		},
	})
}

func TestContainersToServiceStatus(t *testing.T) {
	containers := []types.Container{
		{
			ID:     "c1",
			State:  "running",
			Labels: map[string]string{serviceLabel: "service1"},
		},
		{
			ID:     "c2",
			State:  "exited",
			Labels: map[string]string{serviceLabel: "service1"},
		},
		{
			ID:     "c3",
			State:  "running",
			Labels: map[string]string{serviceLabel: "service1"},
		},
		{
			ID:     "c4",
			State:  "running",
			Labels: map[string]string{serviceLabel: "service2"},
		},
	}
	services, err := containersToServiceStatus(containers)
	assert.NilError(t, err)
	assert.DeepEqual(t, services, []compose.ServiceStatus{
		{
			ID:       "service1",
			Name:     "service1",
			Replicas: 2,
			Desired:  3,
		},
		{
			ID:       "service2",
			Name:     "service2",
			Replicas: 1,
			Desired:  1,
		},
	})
}

func TestStacksMixedStatus(t *testing.T) {
	assert.Equal(t, combinedStatus([]string{"running"}), "running(1)")
	assert.Equal(t, combinedStatus([]string{"running", "running", "running"}), "running(3)")
	assert.Equal(t, combinedStatus([]string{"running", "exited", "running"}), "exited(1), running(2)")
}

func TestBuildBindMount(t *testing.T) {
	volume := composetypes.ServiceVolumeConfig{
		Type:   composetypes.VolumeTypeBind,
		Source: "compose/e2e/volume-test",
		Target: "/data",
	}
	mount, err := buildMount(volume)
	assert.NilError(t, err)
	assert.Assert(t, filepath.IsAbs(mount.Source))
	_, err = os.Stat(mount.Source)
	assert.NilError(t, err)
	assert.Equal(t, mount.Type, mountTypes.TypeBind)
}
