// +build kube

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

package kube

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestContextDescriptionIfEnvVar(t *testing.T) {
	cp := ContextParams{
		FromEnvironment: true,
	}
	description := cp.getDescription()
	assert.Equal(t, description, "From environment variables")
}

func TestContextDescriptionIfProvided(t *testing.T) {
	cp := ContextParams{
		Description:     "custom description",
		FromEnvironment: true,
	}
	description := cp.getDescription()
	assert.Equal(t, description, "custom description")
}

func TestContextDescriptionIfConfigFile(t *testing.T) {
	cp := ContextParams{
		KubeContextName: "my-context",
		KubeConfigPath:  "~/.kube/config",
	}
	description := cp.getDescription()
	assert.Equal(t, description, "my-context (in ~/.kube/config)")
}
func TestContextDescriptionIfDefaultConfigFile(t *testing.T) {
	cp := ContextParams{
		KubeContextName: "my-context",
	}
	description := cp.getDescription()
	assert.Equal(t, description, "my-context (in default kube config)")
}
