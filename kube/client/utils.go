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
	"time"

	"github.com/docker/compose-cli/api/compose"
	corev1 "k8s.io/api/core/v1"
)

func podToContainerSummary(pod corev1.Pod) compose.ContainerSummary {
	return compose.ContainerSummary{
		ID:      pod.GetObjectMeta().GetName(),
		Name:    pod.GetObjectMeta().GetName(),
		Service: pod.GetObjectMeta().GetLabels()[compose.ServiceTag],
		State:   string(pod.Status.Phase),
		Project: pod.GetObjectMeta().GetLabels()[compose.ProjectTag],
	}
}

type LogFunc func(pod string, stateReached bool, message string)

// ServiceStatus hold status about a service
type WaitForStatusOptions struct {
	ProjectName string
	Services    []string
	Status      string
	Timeout     *time.Duration
	Log         LogFunc
}
