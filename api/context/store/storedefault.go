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

package store

import (
	"bytes"
	"encoding/json"
	"os/exec"

	"github.com/pkg/errors"
)

// Represents a context as created by the docker cli
type defaultContext struct {
	Metadata  ContextMetadata
	Endpoints endpoints
}

// Normally (in docker/cli code), the endpoints are mapped as map[string]interface{}
// but docker cli contexts always have a "docker" and "kubernetes" key so we
// create real types for those to no have to juggle around with interfaces.
type endpoints struct {
	Docker     endpoint `json:"docker,omitempty"`
	Kubernetes endpoint `json:"kubernetes,omitempty"`
}

// Both "docker" and "kubernetes" endpoints in the docker cli created contexts
// have a "Host", only kubernetes has the "DefaultNamespace", we put both of
// those here for easier manipulation and to not have to create two distinct
// structs
type endpoint struct {
	Host             string
	DefaultNamespace string
}

func dockerDefaultContext() (*DockerContext, error) {
	// ensure we run this using default context, in current context has been damaged / removed in store
	cmd := exec.Command("com.docker.cli", "--context", "default", "context", "inspect", "default")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var ctx []defaultContext
	err = json.Unmarshal(stdout.Bytes(), &ctx)
	if err != nil {
		return nil, err
	}

	if len(ctx) != 1 {
		return nil, errors.New("found more than one default context")
	}

	defaultCtx := ctx[0]

	meta := DockerContext{
		Name: "default",
		Endpoints: map[string]interface{}{
			"docker": &Endpoint{
				Host: defaultCtx.Endpoints.Docker.Host,
			},
			"kubernetes": &Endpoint{
				Host:             defaultCtx.Endpoints.Kubernetes.Host,
				DefaultNamespace: defaultCtx.Endpoints.Kubernetes.DefaultNamespace,
			},
		},
		Metadata: ContextMetadata{
			Type:              DefaultContextType,
			Description:       "Current DOCKER_HOST based configuration",
			StackOrchestrator: defaultCtx.Metadata.StackOrchestrator,
		},
	}

	return &meta, nil
}
