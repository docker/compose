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

package projectloader

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestServiceOfContainer(t *testing.T) {
	services := types.Services{}
	labelled := container.Summary{Image: "img", Labels: map[string]string{api.ServiceLabel: "web"}}
	name, service := ServiceOfContainer(services, labelled)
	assert.Equal(t, name, "web")
	assert.Equal(t, service.Name, "web")
	assert.Equal(t, service.Image, "img")
	assert.Equal(t, *service.Scale, 1)

	services[name] = service
	_, service = ServiceOfContainer(services, labelled)
	assert.Equal(t, *service.Scale, 2)

	name, _ = ServiceOfContainer(services, container.Summary{Names: []string{"/standalone"}})
	assert.Equal(t, name, "standalone")
}
