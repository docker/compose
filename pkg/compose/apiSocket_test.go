/*
   Copyright 2026 Docker Compose CLI authors

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

// linuxContextInfo is the context of a Linux engine.
type linuxContextInfo struct{}

func (linuxContextInfo) CurrentContext() string         { return "default" }
func (linuxContextInfo) ServerOSType() string           { return "linux" }
func (linuxContextInfo) BuildKitEnabled() (bool, error) { return false, nil }

// TestUseAPISocketIsIdempotent verifies that a project which already went
// through useAPISocket -- the plan's own project, handed to a later plan such
// as a watch rebuild -- is not transformed a second time: the socket and the
// config file would be mounted twice.
func TestUseAPISocketIsIdempotent(t *testing.T) {
	svc, _ := newTestService(t, WithContextInfo(linuxContextInfo{}))
	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"app": {Name: "app", ContainerSpec: types.ContainerSpec{UseAPISocket: true, Environment: types.MappingWithEquals{}}},
		},
		Configs: types.Configs{},
	}

	once, err := svc.useAPISocket(project)
	assert.NilError(t, err)
	service := once.Services["app"]
	assert.Equal(t, len(service.Volumes), 1, "volumes: %v", service.Volumes)
	assert.Equal(t, len(service.Configs), 1, "configs: %v", service.Configs)

	twice, err := svc.useAPISocket(once)
	assert.NilError(t, err)
	service = twice.Services["app"]
	assert.Equal(t, len(service.Volumes), 1, "volumes: %v", service.Volumes)
	assert.Equal(t, len(service.Configs), 1, "configs: %v", service.Configs)
}
