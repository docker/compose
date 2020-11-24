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

package convert

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestComposeSingleContainerRestartPolicy(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					RestartPolicy: &types.RestartPolicy{
						Condition: "on-failure",
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	assert.Assert(t, is.Len(*group.Containers, 1))
	assert.Equal(t, *(*group.Containers)[0].Name, "service1")
	assert.Equal(t, group.RestartPolicy, containerinstance.OnFailure)
}

func TestComposeMultiContainerRestartPolicy(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					RestartPolicy: &types.RestartPolicy{
						Condition: "on-failure",
					},
				},
			},
			{
				Name:  "service2",
				Image: "image2",
				Deploy: &types.DeployConfig{
					RestartPolicy: &types.RestartPolicy{
						Condition: "on-failure",
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	assert.Assert(t, is.Len(*group.Containers, 3))
	assert.Equal(t, *(*group.Containers)[0].Name, "service1")
	assert.Equal(t, group.RestartPolicy, containerinstance.OnFailure)
	assert.Equal(t, *(*group.Containers)[1].Name, "service2")
	assert.Equal(t, group.RestartPolicy, containerinstance.OnFailure)
}

func TestComposeInconsistentMultiContainerRestartPolicy(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					RestartPolicy: &types.RestartPolicy{
						Condition: "any",
					},
				},
			},
			{
				Name:  "service2",
				Image: "image2",
				Deploy: &types.DeployConfig{
					RestartPolicy: &types.RestartPolicy{
						Condition: "on-failure",
					},
				},
			},
		},
	}

	_, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.Error(t, err, "ACI integration does not support specifying different restart policies on services in the same compose application")
}

func TestComposeSingleContainerGroupToContainerDefaultRestartPolicy(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	assert.Assert(t, is.Len(*group.Containers, 1))
	assert.Equal(t, *(*group.Containers)[0].Name, "service1")
	assert.Equal(t, group.RestartPolicy, containerinstance.Always)
}

func TestConvertToAciRestartPolicyCondition(t *testing.T) {
	assert.Equal(t, toAciRestartPolicy("none"), containerinstance.Never)
	assert.Equal(t, toAciRestartPolicy("always"), containerinstance.Always)
	assert.Equal(t, toAciRestartPolicy("on-failure"), containerinstance.OnFailure)
	assert.Equal(t, toAciRestartPolicy("on-failure:5"), containerinstance.Always)
}

func TestConvertToDockerRestartPolicyCondition(t *testing.T) {
	assert.Equal(t, toContainerRestartPolicy(containerinstance.Never), "none")
	assert.Equal(t, toContainerRestartPolicy(containerinstance.Always), "any")
	assert.Equal(t, toContainerRestartPolicy(containerinstance.OnFailure), "on-failure")
	assert.Equal(t, toContainerRestartPolicy(""), "any")
}
