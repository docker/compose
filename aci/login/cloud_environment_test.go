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

package login

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestNormalizeCloudEnvironmentURLs(t *testing.T) {
	ce := CloudEnvironment{
		Name: "SecretCloud",
		Authentication: CloudEnvironmentAuthentication{
			LoginEndpoint: "https://login.here.com/",
			Audiences: []string{
				"https://audience1",
				"https://audience2/",
			},
			Tenant: "common",
		},
		ResourceManagerURL: "invalid URL",
	}

	ce.normalizeURLs()

	assert.Equal(t, ce.Authentication.LoginEndpoint, "https://login.here.com")
	assert.Equal(t, ce.Authentication.Audiences[0], "https://audience1")
	assert.Equal(t, ce.Authentication.Audiences[1], "https://audience2")
}

func TestApplyInvalidCloudMetadataJSON(t *testing.T) {
	ce := newCloudEnvironmentService()
	bb := []byte(`This isn't really valid JSON`)

	err := ce.applyCloudMetadata(bb)

	assert.Assert(t, err != nil, "Cloud metadata was invalid, so the application should have failed")
	ensureWellKnownCloudMetadata(t, ce)
}

func TestApplyInvalidCloudMetatada(t *testing.T) {
	ce := newCloudEnvironmentService()

	// No name (moniker) for the cloud
	bb := []byte(`
	[{
		"authentication": {
			"loginEndpoint": "https://login.docker.com/",
			"audiences": [
				"https://management.docker.com/",
				"https://management.cli.docker.com/"
			],
			"tenant": "F5773994-FE88-482E-9E33-6E799D250416"
		},
		"suffixes": {
			"acrLoginServer": "azurecr.docker.io"
		},
		"resourceManager": "https://management.docker.com/"
	}]`)

	err := ce.applyCloudMetadata(bb)
	assert.ErrorContains(t, err, "no name")
	ensureWellKnownCloudMetadata(t, ce)

	// Invalid resource manager URL
	bb = []byte(`
	[{
		"authentication": {
			"loginEndpoint": "https://login.docker.com/",
			"audiences": [
				"https://management.docker.com/",
				"https://management.cli.docker.com/"
			],
			"tenant": "F5773994-FE88-482E-9E33-6E799D250416"
		},
		"name": "DockerAzureCloud",
		"suffixes": {
			"acrLoginServer": "azurecr.docker.io"
		},
		"resourceManager": "123"
	}]`)

	err = ce.applyCloudMetadata(bb)
	assert.ErrorContains(t, err, "invalid resource manager URL")
	ensureWellKnownCloudMetadata(t, ce)

	// Invalid login endpoint
	bb = []byte(`
	[{
		"authentication": {
			"loginEndpoint": "456",
			"audiences": [
				"https://management.docker.com/",
				"https://management.cli.docker.com/"
			],
			"tenant": "F5773994-FE88-482E-9E33-6E799D250416"
		},
		"name": "DockerAzureCloud",
		"suffixes": {
			"acrLoginServer": "azurecr.docker.io"
		},
		"resourceManager": "https://management.docker.com/"
	}]`)

	err = ce.applyCloudMetadata(bb)
	assert.ErrorContains(t, err, "invalid login endpoint")
	ensureWellKnownCloudMetadata(t, ce)

	// No audiences
	bb = []byte(`
	[{
		"authentication": {
			"loginEndpoint": "https://login.docker.com/",
			"audiences": [ ],
			"tenant": "F5773994-FE88-482E-9E33-6E799D250416"
		},
		"name": "DockerAzureCloud",
		"suffixes": {
			"acrLoginServer": "azurecr.docker.io"
		},
		"resourceManager": "https://management.docker.com/"
	}]`)

	err = ce.applyCloudMetadata(bb)
	assert.ErrorContains(t, err, "no authentication audiences")
	ensureWellKnownCloudMetadata(t, ce)
}

func TestApplyCloudMetadata(t *testing.T) {
	ce := newCloudEnvironmentService()

	bb := []byte(`
	[{
		"authentication": {
			"loginEndpoint": "https://login.docker.com/",
			"audiences": [
				"https://management.docker.com/",
				"https://management.cli.docker.com/"
			],
			"tenant": "F5773994-FE88-482E-9E33-6E799D250416"
		},
		"name": "DockerAzureCloud",
		"suffixes": {
			"acrLoginServer": "azurecr.docker.io"
		},
		"resourceManager": "https://management.docker.com/"
	}]`)

	err := ce.applyCloudMetadata(bb)
	assert.NilError(t, err)

	env, err := ce.Get("DockerAzureCloud")
	assert.NilError(t, err)
	assert.Equal(t, env.Authentication.LoginEndpoint, "https://login.docker.com")
	ensureWellKnownCloudMetadata(t, ce)
}

func TestDefaultCloudMetadataPresent(t *testing.T) {
	ensureWellKnownCloudMetadata(t, CloudEnvironments)
}

func ensureWellKnownCloudMetadata(t *testing.T, ce CloudEnvironmentService) {
	// Make sure well-known public cloud information is still available
	_, err := ce.Get(AzurePublicCloudName)
	assert.NilError(t, err)

	_, err = ce.Get("AzureChinaCloud")
	assert.NilError(t, err)

	_, err = ce.Get("AzureUSGovernment")
	assert.NilError(t, err)
}
