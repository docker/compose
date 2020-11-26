// +build local

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

package local

import (
	"fmt"

	"github.com/docker/docker/api/types/filters"
)

const (
	containerNumberLabel = "com.docker.compose.container-number"
	oneoffLabel          = "com.docker.compose.oneoff"
	projectLabel         = "com.docker.compose.project"
	workingDirLabel      = "com.docker.compose.project.working_dir"
	configFilesLabel     = "com.docker.compose.project.config_files"
	serviceLabel         = "com.docker.compose.service"
	versionLabel         = "com.docker.compose.version"
	configHashLabel      = "com.docker.compose.config-hash"
	networkLabel         = "com.docker.compose.network"

	//ComposeVersion Compose version
	ComposeVersion = "1.0-alpha"
)

func projectFilter(projectName string) filters.KeyValuePair {
	return filters.Arg("label", fmt.Sprintf("%s=%s", projectLabel, projectName))
}

func hasProjectLabelFilter() filters.KeyValuePair {
	return filters.Arg("label", projectLabel)
}
