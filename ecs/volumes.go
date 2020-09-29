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
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) createNFSmountIngress(securityGroups []string, project *types.Project, n string, template *cloudformation.Template) error {
	target := securityGroups[0]
	for _, s := range project.Services {
		for _, v := range s.Volumes {
			if v.Source != n {
				continue
			}
			var source string
			for net := range s.Networks {
				network := project.Networks[net]
				if ext, ok := network.Extensions[extensionSecurityGroup]; ok {
					source = ext.(string)
				} else {
					source = networkResourceName(net)
				}
				break
			}
			name := fmt.Sprintf("%sNFSMount%s", s.Name, n)
			template.Resources[name] = &ec2.SecurityGroupIngress{
				Description:           fmt.Sprintf("Allow NFS mount for %s on %s", s.Name, n),
				GroupId:               target,
				SourceSecurityGroupId: cloudformation.Ref(source),
				IpProtocol:            "tcp",
				FromPort:              2049,
				ToPort:                2049,
			}
			service := template.Resources[serviceResourceName(s.Name)].(*ecs.Service)
			service.AWSCloudFormationDependsOn = append(service.AWSCloudFormationDependsOn, name)
		}
	}
	return nil
}
