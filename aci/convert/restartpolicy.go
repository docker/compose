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
	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/containers"
)

func (p projectAciHelper) getRestartPolicy() (containerinstance.ContainerGroupRestartPolicy, error) {
	var restartPolicyCondition containerinstance.ContainerGroupRestartPolicy
	if len(p.Services) >= 1 {
		alreadySpecified := false
		restartPolicyCondition = containerinstance.Always
		for _, service := range p.Services {
			if service.Deploy != nil &&
				service.Deploy.RestartPolicy != nil {
				if !alreadySpecified {
					alreadySpecified = true
					restartPolicyCondition = toAciRestartPolicy(service.Deploy.RestartPolicy.Condition)
				}
				if alreadySpecified && restartPolicyCondition != toAciRestartPolicy(service.Deploy.RestartPolicy.Condition) {
					return "", errors.New("ACI integration does not support specifying different restart policies on services in the same compose application")
				}

			}
		}
	}
	return restartPolicyCondition, nil
}

func toAciRestartPolicy(restartPolicy string) containerinstance.ContainerGroupRestartPolicy {
	switch restartPolicy {
	case containers.RestartPolicyNone:
		return containerinstance.Never
	case containers.RestartPolicyAny:
		return containerinstance.Always
	case containers.RestartPolicyOnFailure:
		return containerinstance.OnFailure
	default:
		return containerinstance.Always
	}
}

func toContainerRestartPolicy(aciRestartPolicy containerinstance.ContainerGroupRestartPolicy) string {
	switch aciRestartPolicy {
	case containerinstance.Never:
		return containers.RestartPolicyNone
	case containerinstance.Always:
		return containers.RestartPolicyAny
	case containerinstance.OnFailure:
		return containers.RestartPolicyOnFailure
	default:
		return containers.RestartPolicyAny
	}
}
