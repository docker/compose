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

package compose

import (
	"fmt"
	"strconv"

	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
)

// labelFilter returns a label filter string of the form "key=value".
func labelFilter(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

func projectFilter(projectName string) client.Filters {
	return make(client.Filters).Add("label", labelFilter(api.ProjectLabel, projectName))
}

func serviceFilter(serviceName string) string {
	return labelFilter(api.ServiceLabel, serviceName)
}

func networkFilter(name string) string {
	return labelFilter(api.NetworkLabel, name)
}

func oneOffFilter(b bool) string {
	v := "False"
	if b {
		v = "True"
	}
	return labelFilter(api.OneoffLabel, v)
}

func containerNumberFilter(index int) string {
	return labelFilter(api.ContainerNumberLabel, strconv.Itoa(index))
}
