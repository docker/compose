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

package ecs

import (
	"fmt"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/efs"
	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) createNFSMountTarget(project *types.Project, resources awsResources, template *cloudformation.Template) {
	for volume := range project.Volumes {
		for _, subnet := range resources.subnets {
			name := fmt.Sprintf("%sNFSMountTargetOn%s", normalizeResourceName(volume), normalizeResourceName(subnet))
			template.Resources[name] = &efs.MountTarget{
				FileSystemId:   resources.filesystems[volume],
				SecurityGroups: resources.allSecurityGroups(),
				SubnetId:       subnet,
			}
		}
	}
}

func (b *ecsAPIService) mountTargets(volume string, resources awsResources) []string {
	var refs []string
	for _, subnet := range resources.subnets {
		refs = append(refs, fmt.Sprintf("%sNFSMountTargetOn%s", normalizeResourceName(volume), normalizeResourceName(subnet)))
	}
	return refs
}
