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

func TestServiceHashWithAllValuesTheSame(t *testing.T) {
	hash1, err := ServiceHash(serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	hash2, err := ServiceHash(serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	assert.Equal(t, hash1, hash2)
}

func TestServiceHashWithIgnorableValues(t *testing.T) {
	hash1, err := ServiceHash(serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	hash2, err := ServiceHash(serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, hash1, hash2)
}

func TestServiceDependenciesHashWithoutChangesContent(t *testing.T) {
	hash1, err := ServiceDependenciesHash(projectConfig("myConfigSource", "a", "", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	hash2, err := ServiceDependenciesHash(projectConfig("myConfigSource", "a", "", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Assert(t, hash1 == hash2)
}

func TestServiceDependenciesHashWithChangedConfigContent(t *testing.T) {
	hash1, err := ServiceDependenciesHash(projectConfig("myConfigSource", "a", "", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	hash2, err := ServiceDependenciesHash(projectConfig("myConfigSource", "b", "", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Assert(t, hash1 != hash2)
}

func TestServiceDependenciesHashWithChangedConfigEnvironment(t *testing.T) {
	hash1, err := ServiceDependenciesHash(projectConfig("myConfigSource", "", "a", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	hash2, err := ServiceDependenciesHash(projectConfig("myConfigSource", "", "b", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Assert(t, hash1 != hash2)
}

func TestServiceDependenciesHashWithChangedConfigFile(t *testing.T) {
	hash1, err := ServiceDependenciesHash(
		projectConfig("myConfigSource", "", "", "./testdata/config1.txt"),
		serviceConfig("myContext1", "always", 1),
	)
	assert.NilError(t, err)
	hash2, err := ServiceDependenciesHash(
		projectConfig("myConfigSource", "", "", "./testdata/config2.txt"),
		serviceConfig("myContext2", "never", 2),
	)
	assert.NilError(t, err)
	assert.Assert(t, hash1 != hash2)
}

func projectConfig(configName, configContent, configEnvironment, configFile string) *types.Project {
	return &types.Project{
		Configs: types.Configs{
			configName: types.ConfigObjConfig{
				Content:     configContent,
				Environment: configEnvironment,
				File:        configFile,
			},
		},
	}
}

func serviceConfig(buildContext, pullPolicy string, replicas int) types.ServiceConfig {
	return types.ServiceConfig{
		Build: &types.BuildConfig{
			Context: buildContext,
		},
		PullPolicy: pullPolicy,
		Scale:      &replicas,
		Deploy: &types.DeployConfig{
			Replicas: &replicas,
		},
		Name:  "foo",
		Image: "bar",
		Configs: []types.ServiceConfigObjConfig{
			{
				Source: "myConfigSource",
			},
		},
	}
}
