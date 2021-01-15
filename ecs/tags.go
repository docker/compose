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
	"github.com/awslabs/goformation/v4/cloudformation/tags"
	"github.com/compose-spec/compose-go/types"

	"github.com/docker/compose-cli/api/compose"
)

func projectTags(project *types.Project) []tags.Tag {
	return []tags.Tag{
		{
			Key:   compose.ProjectTag,
			Value: project.Name,
		},
	}
}

func serviceTags(project *types.Project, service types.ServiceConfig) []tags.Tag {
	return []tags.Tag{
		{
			Key:   compose.ProjectTag,
			Value: project.Name,
		},
		{
			Key:   compose.ServiceTag,
			Value: service.Name,
		},
	}
}

func networkTags(project *types.Project, net types.NetworkConfig) []tags.Tag {
	return []tags.Tag{
		{
			Key:   compose.ProjectTag,
			Value: project.Name,
		},
		{
			Key:   compose.NetworkTag,
			Value: net.Name,
		},
	}
}
