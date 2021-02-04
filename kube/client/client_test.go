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

package client

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/api/compose"
)

func TestPodToContainerSummary(t *testing.T) {
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c1-123",
			Labels: map[string]string{
				compose.ProjectTag: "myproject",
				compose.ServiceTag: "service1",
			},
		},
		Status: v1.PodStatus{
			Phase: "Running",
		},
	}

	container := podToContainerSummary(pod)

	expected := compose.ContainerSummary{
		ID:      "c1-123",
		Name:    "c1-123",
		Project: "myproject",
		Service: "service1",
		State:   "Running",
	}
	assert.DeepEqual(t, container, expected)
}
