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
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/context/store"
)

var (
	convertCtx = store.AciContext{
		SubscriptionID: "subID",
		ResourceGroup:  "rg",
		Location:       "eu",
	}
	mockStorageHelper = &mockStorageLogin{}
)

func TestProjectName(t *testing.T) {
	project := types.Project{
		Name: "TEST",
	}
	containerGroup, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.Equal(t, *containerGroup.Name, "test")
}

func TestContainerGroupToContainer(t *testing.T) {
	myContainerGroup := containerinstance.ContainerGroup{
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			IPAddress: &containerinstance.IPAddress{
				Ports: &[]containerinstance.Port{{
					Port: to.Int32Ptr(80),
				}},
				IP:           to.StringPtr("42.42.42.42"),
				DNSNameLabel: to.StringPtr("myapp"),
			},
			OsType: "Linux",
		},
	}
	myContainer := containerinstance.Container{
		Name: to.StringPtr("myContainerID"),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image:   to.StringPtr("sha256:666"),
			Command: to.StringSlicePtr([]string{"mycommand"}),
			Ports: &[]containerinstance.ContainerPort{{
				Port: to.Int32Ptr(80),
			}},
			EnvironmentVariables: nil,
			InstanceView: &containerinstance.ContainerPropertiesInstanceView{
				RestartCount: nil,
				CurrentState: &containerinstance.ContainerState{
					State: to.StringPtr("Running"),
				},
			},
			Resources: &containerinstance.ResourceRequirements{
				Limits: &containerinstance.ResourceLimits{
					CPU:        to.Float64Ptr(3),
					MemoryInGB: to.Float64Ptr(0.2),
				},
				Requests: &containerinstance.ResourceRequests{
					CPU:        to.Float64Ptr(2),
					MemoryInGB: to.Float64Ptr(0.1),
				},
			},
			LivenessProbe: &containerinstance.ContainerProbe{
				Exec: &containerinstance.ContainerExec{
					Command: to.StringSlicePtr([]string{
						"my",
						"command",
						"--option",
					}),
				},
				PeriodSeconds:       to.Int32Ptr(10),
				FailureThreshold:    to.Int32Ptr(3),
				InitialDelaySeconds: to.Int32Ptr(2),
				TimeoutSeconds:      to.Int32Ptr(1),
			},
		},
	}

	var expectedContainer = containers.Container{
		ID:       "myContainerID",
		Status:   "Running",
		Image:    "sha256:666",
		Command:  "mycommand",
		Platform: "Linux",
		Ports: []containers.Port{{
			HostPort:      uint32(80),
			ContainerPort: uint32(80),
			Protocol:      "tcp",
			HostIP:        "42.42.42.42",
		}},
		Config: &containers.RuntimeConfig{
			FQDN: "myapp.eastus.azurecontainer.io",
		},
		HostConfig: &containers.HostConfig{
			CPULimit:          3,
			CPUReservation:    2,
			MemoryLimit:       gbToBytes(0.2),
			MemoryReservation: gbToBytes(0.1),
			RestartPolicy:     "any",
		},
		Healthcheck: containers.Healthcheck{
			Disable: false,
			Test: []string{
				"my",
				"command",
				"--option",
			},
			Interval:    types.Duration(10 * time.Second),
			Retries:     3,
			StartPeriod: types.Duration(2 * time.Second),
			Timeout:     types.Duration(time.Second),
		},
	}

	container := ContainerGroupToContainer("myContainerID", myContainerGroup, myContainer, "eastus")
	assert.DeepEqual(t, container, expectedContainer)
}

func TestHealthcheckTranslation(t *testing.T) {
	test := []string{
		"my",
		"command",
		"--option",
	}
	interval := types.Duration(10 * time.Second)
	retries := uint64(42)
	startPeriod := types.Duration(2 * time.Second)
	timeout := types.Duration(3 * time.Second)
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				HealthCheck: &types.HealthCheckConfig{
					Test:        test,
					Timeout:     &timeout,
					Interval:    &interval,
					Retries:     &retries,
					StartPeriod: &startPeriod,
					Disable:     false,
				},
			},
		},
	}

	testHealthcheckTestPrefixRemoval := func(test []string, shellPreffix ...string) {
		project.Services[0].HealthCheck.Test = append(shellPreffix, test...)
		group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
		assert.NilError(t, err)
		assert.DeepEqual(t, (*group.Containers)[0].LivenessProbe.Exec.Command, to.StringSlicePtr(test))
		assert.Equal(t, *(*group.Containers)[0].LivenessProbe.PeriodSeconds, int32(10))
		assert.Assert(t, (*group.Containers)[0].LivenessProbe.SuccessThreshold == nil)
		assert.Equal(t, *(*group.Containers)[0].LivenessProbe.FailureThreshold, int32(42))
		assert.Equal(t, *(*group.Containers)[0].LivenessProbe.InitialDelaySeconds, int32(2))
		assert.Equal(t, *(*group.Containers)[0].LivenessProbe.TimeoutSeconds, int32(3))
	}

	testHealthcheckTestPrefixRemoval(test)
	testHealthcheckTestPrefixRemoval(test, "NONE")
	testHealthcheckTestPrefixRemoval(test, "CMD")
	testHealthcheckTestPrefixRemoval(test, "CMD-SHELL")

	project.Services[0].HealthCheck.Disable = true
	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.Assert(t, (*group.Containers)[0].LivenessProbe == nil)
}

func TestHealthcheckTranslationZeroValues(t *testing.T) {
	test := []string{
		"my",
		"command",
		"--option",
	}
	interval := types.Duration(0)
	retries := uint64(0)
	startPeriod := types.Duration(0)
	timeout := types.Duration(0)

	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				HealthCheck: &types.HealthCheckConfig{
					Test:        test,
					Timeout:     &timeout,
					Interval:    &interval,
					Retries:     &retries,
					StartPeriod: &startPeriod,
					Disable:     false,
				},
			},
		},
	}
	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.DeepEqual(t, (*group.Containers)[0].LivenessProbe.Exec.Command, to.StringSlicePtr(test))
	assert.Assert(t, (*group.Containers)[0].LivenessProbe.PeriodSeconds == nil)
	assert.Assert(t, (*group.Containers)[0].LivenessProbe.SuccessThreshold == nil)
	assert.Assert(t, (*group.Containers)[0].LivenessProbe.FailureThreshold == nil)
	assert.Assert(t, (*group.Containers)[0].LivenessProbe.InitialDelaySeconds == nil)
	assert.Assert(t, (*group.Containers)[0].LivenessProbe.TimeoutSeconds == nil)
}

func TestContainerGroupToServiceStatus(t *testing.T) {
	myContainerGroup := containerinstance.ContainerGroup{
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			IPAddress: &containerinstance.IPAddress{
				Ports: &[]containerinstance.Port{{
					Port: to.Int32Ptr(80),
				}},
				IP: to.StringPtr("42.42.42.42"),
			},
		},
	}
	myContainer := containerinstance.Container{
		Name: to.StringPtr("myContainerID"),
		ContainerProperties: &containerinstance.ContainerProperties{
			Ports: &[]containerinstance.ContainerPort{{
				Port: to.Int32Ptr(80),
			}},
			InstanceView: &containerinstance.ContainerPropertiesInstanceView{
				RestartCount: nil,
				CurrentState: &containerinstance.ContainerState{
					State: to.StringPtr("Running"),
				},
			},
		},
	}

	var expectedService = compose.ServiceStatus{
		ID:       "myContainerID",
		Name:     "myContainerID",
		Ports:    []string{"42.42.42.42:80->80/tcp"},
		Replicas: 1,
		Desired:  1,
	}

	container := ContainerGroupToServiceStatus("myContainerID", myContainerGroup, myContainer, "eastus")
	assert.DeepEqual(t, container, expectedService)
}

func TestComposeContainerGroupToContainerWithDnsSideCarSide(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
			{
				Name:  "service2",
				Image: "image2",
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.Assert(t, is.Len(*group.Containers, 3))

	assert.Equal(t, *(*group.Containers)[0].Name, "service1")
	assert.Equal(t, *(*group.Containers)[1].Name, "service2")
	assert.Equal(t, *(*group.Containers)[2].Name, ComposeDNSSidecarName)

	assert.DeepEqual(t, *(*group.Containers)[2].Command, []string{"/hosts", "service1", "service2"})

	assert.Equal(t, *(*group.Containers)[0].Image, "image1")
	assert.Equal(t, *(*group.Containers)[1].Image, "image2")
	assert.Equal(t, *(*group.Containers)[2].Image, dnsSidecarImage)
}

func TestComposeSingleContainerGroupToContainerNoDnsSideCarSide(t *testing.T) {
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
	assert.Equal(t, *(*group.Containers)[0].Image, "image1")
}

func TestLabelsErrorMessage(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Labels: map[string]string{
					"label1": "value1",
				},
			},
		},
	}

	_, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.Error(t, err, "ACI integration does not support labels in compose applications")
}

func TestComposeContainerGroupToContainerWithDomainName(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Ports: []types.ServicePortConfig{
					{
						Published: 80,
						Target:    80,
					},
				},
				DomainName: "myApp",
			},
			{
				Name:  "service2",
				Image: "image2",
				Ports: []types.ServicePortConfig{
					{
						Published: 8080,
						Target:    8080,
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.Assert(t, is.Len(*group.Containers, 3))

	groupPorts := *group.IPAddress.Ports
	assert.Assert(t, is.Len(groupPorts, 2))
	assert.Equal(t, *groupPorts[0].Port, int32(80))
	assert.Equal(t, *groupPorts[1].Port, int32(8080))
	assert.Equal(t, *group.IPAddress.DNSNameLabel, "myApp")
}

func TestComposeContainerGroupToContainerErrorWhenSeveralDomainNames(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:       "service1",
				Image:      "image1",
				DomainName: "myApp",
			},
			{
				Name:       "service2",
				Image:      "image2",
				DomainName: "myApp2",
			},
		},
	}

	_, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.Error(t, err, "ACI integration does not support specifying different domain names on services in the same compose application")
}

// ACI fails if group definition IPAddress has no ports
func TestComposeContainerGroupToContainerIgnoreDomainNameWithoutPorts(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:       "service1",
				Image:      "image1",
				DomainName: "myApp",
			},
			{
				Name:       "service2",
				Image:      "image2",
				DomainName: "myApp",
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.Assert(t, is.Len(*group.Containers, 3))
	assert.Assert(t, group.IPAddress == nil)
}

var _0_1Gb = gbToBytes(0.1)

func TestComposeContainerGroupToContainerResourceRequests(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs:    "0.1",
							MemoryBytes: types.UnitBytes(_0_1Gb),
						},
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	request := *((*group.Containers)[0]).Resources.Requests
	assert.Equal(t, *request.CPU, float64(0.1))
	assert.Equal(t, *request.MemoryInGB, float64(0.1))
	limits := *((*group.Containers)[0]).Resources.Limits
	assert.Equal(t, *limits.CPU, float64(0.1))
	assert.Equal(t, *limits.MemoryInGB, float64(0.1))
}

func TestComposeContainerGroupToContainerResourceRequestsAndLimits(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs:    "0.1",
							MemoryBytes: types.UnitBytes(_0_1Gb),
						},
						Limits: &types.Resource{
							NanoCPUs:    "0.3",
							MemoryBytes: types.UnitBytes(2 * _0_1Gb),
						},
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	request := *((*group.Containers)[0]).Resources.Requests
	assert.Equal(t, *request.CPU, float64(0.1))
	assert.Equal(t, *request.MemoryInGB, float64(0.1))
	limits := *((*group.Containers)[0]).Resources.Limits
	assert.Equal(t, *limits.CPU, float64(0.3))
	assert.Equal(t, *limits.MemoryInGB, float64(0.2))
}

func TestComposeContainerGroupToContainerResourceLimitsOnly(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Limits: &types.Resource{
							NanoCPUs:    "0.3",
							MemoryBytes: types.UnitBytes(2 * _0_1Gb),
						},
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	request := *((*group.Containers)[0]).Resources.Requests
	assert.Equal(t, *request.CPU, float64(0.3))
	assert.Equal(t, *request.MemoryInGB, float64(0.2))
	limits := *((*group.Containers)[0]).Resources.Limits
	assert.Equal(t, *limits.CPU, float64(0.3))
	assert.Equal(t, *limits.MemoryInGB, float64(0.2))
}

func TestComposeContainerGroupToContainerResourceRequestsDefaults(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs:    "",
							MemoryBytes: 0,
						},
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	request := *((*group.Containers)[0]).Resources.Requests
	assert.Equal(t, *request.CPU, float64(1))
	assert.Equal(t, *request.MemoryInGB, float64(1))
}

func TestComposeContainerGroupToContainerenvVar(t *testing.T) {
	err := os.Setenv("key2", "value2")
	assert.NilError(t, err)
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Environment: types.MappingWithEquals{
					"key1": to.StringPtr("value1"),
					"key2": nil,
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)

	envVars := *((*group.Containers)[0]).EnvironmentVariables
	assert.Assert(t, is.Len(envVars, 2))
	assert.Assert(t, is.Contains(envVars, containerinstance.EnvironmentVariable{Name: to.StringPtr("key1"), Value: to.StringPtr("value1")}))
	assert.Assert(t, is.Contains(envVars, containerinstance.EnvironmentVariable{Name: to.StringPtr("key2"), Value: to.StringPtr("value2")}))
}

func TestConvertContainerGroupStatus(t *testing.T) {
	assert.Equal(t, "Running", GetStatus(container(to.StringPtr("Running")), group(to.StringPtr("Started"))))
	assert.Equal(t, "Terminated", GetStatus(container(to.StringPtr("Terminated")), group(to.StringPtr("Stopped"))))
	assert.Equal(t, "Node Stopped", GetStatus(container(nil), group(to.StringPtr("Stopped"))))
	assert.Equal(t, "Node Started", GetStatus(container(nil), group(to.StringPtr("Started"))))

	assert.Equal(t, "Running", GetStatus(container(to.StringPtr("Running")), group(nil)))
	assert.Equal(t, "Unknown", GetStatus(container(nil), group(nil)))
}

func container(status *string) containerinstance.Container {
	var state *containerinstance.ContainerState
	if status != nil {
		state = &containerinstance.ContainerState{
			State: status,
		}
	}
	return containerinstance.Container{
		ContainerProperties: &containerinstance.ContainerProperties{
			InstanceView: &containerinstance.ContainerPropertiesInstanceView{
				CurrentState: state,
			},
		},
	}
}

func group(status *string) containerinstance.ContainerGroup {
	var view *containerinstance.ContainerGroupPropertiesInstanceView
	if status != nil {
		view = &containerinstance.ContainerGroupPropertiesInstanceView{
			State: status,
		}
	}
	return containerinstance.ContainerGroup{
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			InstanceView: view,
		},
	}
}
