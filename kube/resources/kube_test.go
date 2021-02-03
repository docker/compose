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

package resources

import (
	"testing"

	"gotest.tools/v3/assert"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceWithExposedPort(t *testing.T) {
	model, err := loadYAML(`
services:
  nginx:
    image: nginx
    ports:
      - "80:80"
`)
	assert.NilError(t, err)

	service := mapToService(model, model.Services[0])
	assert.DeepEqual(t, *service, core.Service{
		TypeMeta: meta.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: meta.ObjectMeta{
			Name: "nginx",
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"com.docker.compose.service": "nginx", "com.docker.compose.project": ""},
			Ports: []core.ServicePort{
				{
					Name:       "80-tcp",
					Port:       int32(80),
					TargetPort: intstr.FromInt(int(80)),
					Protocol:   core.ProtocolTCP,
				},
			},
			Type: core.ServiceTypeLoadBalancer,
		}})
}

func TestServiceWithoutExposedPort(t *testing.T) {
	model, err := loadYAML(`
services:
  nginx:
    image: nginx
`)
	assert.NilError(t, err)

	service := mapToService(model, model.Services[0])
	assert.DeepEqual(t, *service, core.Service{
		TypeMeta: meta.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: meta.ObjectMeta{
			Name: "nginx",
		},
		Spec: core.ServiceSpec{
			Selector:  map[string]string{"com.docker.compose.service": "nginx", "com.docker.compose.project": ""},
			ClusterIP: "None",
			Ports:     []core.ServicePort{},
			Type:      core.ServiceTypeClusterIP,
		}})
}
