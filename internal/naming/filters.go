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

package naming

import (
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
)

// LabelFilter returns a label filter string of the form "key=value".
func LabelFilter(key, value string) string {
	return key + "=" + value
}

// ProjectFilter selects the resources of a Compose project.
func ProjectFilter(projectName string) client.Filters {
	return make(client.Filters).Add("label", LabelFilter(api.ProjectLabel, projectName))
}

// ServiceFilter returns the label filter selecting the resources of a service.
func ServiceFilter(serviceName string) string {
	return LabelFilter(api.ServiceLabel, serviceName)
}
