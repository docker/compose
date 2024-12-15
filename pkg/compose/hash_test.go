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

func TestServiceConfigsHashWithoutChangesContent(t *testing.T) {
	serviceNameToConfigHash1, err := ServiceConfigsHash(projectWithConfigs("a", "", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	serviceNameToConfigHas2, err := ServiceConfigsHash(projectWithConfigs("a", "", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToConfigHash1), len(serviceNameToConfigHas2))

	for serviceName, hash := range serviceNameToConfigHash1 {
		assert.Equal(t, hash, serviceNameToConfigHas2[serviceName])
	}
}

func TestServiceConfigsHashWithChangedConfigContent(t *testing.T) {
	serviceNameToConfigHash1, err := ServiceConfigsHash(projectWithConfigs("a", "", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	serviceNameToConfigHash2, err := ServiceConfigsHash(projectWithConfigs("b", "", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToConfigHash1), len(serviceNameToConfigHash2))

	for serviceName, hash := range serviceNameToConfigHash1 {
		assert.Assert(t, hash != serviceNameToConfigHash2[serviceName])
	}
}

func TestServiceConfigsHashWithChangedConfigEnvironment(t *testing.T) {
	serviceNameToConfigHash1, err := ServiceConfigsHash(projectWithConfigs("", "a", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	serviceNameToConfigHash2, err := ServiceConfigsHash(projectWithConfigs("", "b", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToConfigHash1), len(serviceNameToConfigHash2))

	for serviceName, hash := range serviceNameToConfigHash1 {
		assert.Assert(t, hash != serviceNameToConfigHash2[serviceName])
	}
}

func TestServiceConfigsHashWithChangedConfigFile(t *testing.T) {
	serviceNameToConfigHash1, err := ServiceConfigsHash(
		projectWithConfigs("", "", "./testdata/config1.txt"),
		serviceConfig("myContext1", "always", 1),
	)
	assert.NilError(t, err)
	serviceNameToConfigHash2, err := ServiceConfigsHash(
		projectWithConfigs("", "", "./testdata/config2.txt"),
		serviceConfig("myContext2", "never", 2),
	)
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToConfigHash1), len(serviceNameToConfigHash2))

	for serviceName, hash := range serviceNameToConfigHash1 {
		assert.Assert(t, hash != serviceNameToConfigHash2[serviceName])
	}
}

func TestServiceSecretsHashWithoutChangesContent(t *testing.T) {
	serviceNameToSecretHash1, err := ServiceSecretsHash(projectWithSecrets("a", "", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	serviceNameToSecretHash2, err := ServiceSecretsHash(projectWithSecrets("a", "", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToSecretHash1), len(serviceNameToSecretHash2))

	for serviceName, hash := range serviceNameToSecretHash1 {
		assert.Equal(t, hash, serviceNameToSecretHash2[serviceName])
	}
}

func TestServiceSecretsHashWithChangedSecretContent(t *testing.T) {
	serviceNameToSecretHash1, err := ServiceSecretsHash(projectWithSecrets("a", "", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	serviceNameToSecretHash2, err := ServiceSecretsHash(projectWithSecrets("b", "", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToSecretHash1), len(serviceNameToSecretHash2))

	for serviceName, hash := range serviceNameToSecretHash1 {
		assert.Assert(t, hash != serviceNameToSecretHash2[serviceName])
	}
}

func TestServiceSecretsHashWithChangedSecretEnvironment(t *testing.T) {
	serviceNameToSecretHash1, err := ServiceSecretsHash(projectWithSecrets("", "a", ""), serviceConfig("myContext1", "always", 1))
	assert.NilError(t, err)
	serviceNameToSecretHash2, err := ServiceSecretsHash(projectWithSecrets("", "b", ""), serviceConfig("myContext2", "never", 2))
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToSecretHash1), len(serviceNameToSecretHash2))

	for serviceName, hash := range serviceNameToSecretHash1 {
		assert.Assert(t, hash != serviceNameToSecretHash2[serviceName])
	}
}

func TestServiceSecretsHashWithChangedSecretFile(t *testing.T) {
	serviceNameToSecretHash1, err := ServiceSecretsHash(
		projectWithSecrets("", "", "./testdata/config1.txt"),
		serviceConfig("myContext1", "always", 1),
	)
	assert.NilError(t, err)
	serviceNameToSecretHash2, err := ServiceSecretsHash(
		projectWithSecrets("", "", "./testdata/config2.txt"),
		serviceConfig("myContext2", "never", 2),
	)
	assert.NilError(t, err)
	assert.Equal(t, len(serviceNameToSecretHash1), len(serviceNameToSecretHash2))

	for serviceName, hash := range serviceNameToSecretHash1 {
		assert.Assert(t, hash != serviceNameToSecretHash2[serviceName])
	}
}

func projectWithConfigs(configContent, configEnvironmentValue, configFile string) *types.Project {
	envName := "myEnv"

	if configEnvironmentValue == "" {
		envName = ""
	}

	return &types.Project{
		Environment: types.Mapping{
			envName: configEnvironmentValue,
		},
		Configs: types.Configs{
			"myConfigSource": types.ConfigObjConfig{
				Content:     configContent,
				Environment: envName,
				File:        configFile,
			},
		},
	}
}

func projectWithSecrets(secretContent, secretEnvironmentValue, secretFile string) *types.Project {
	envName := "myEnv"

	if secretEnvironmentValue == "" {
		envName = ""
	}

	return &types.Project{
		Environment: types.Mapping{
			envName: secretEnvironmentValue,
		},
		Secrets: types.Secrets{
			"mySecretSource": types.SecretConfig{
				Content:     secretContent,
				Environment: envName,
				File:        secretFile,
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
		Secrets: []types.ServiceSecretConfig{
			{
				Source: "mySecretSource",
			},
		},
	}
}
