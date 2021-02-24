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
	"os"
	"strings"

	pluginmanager "github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/command"
)

func displayScanSuggestMsg(builtImages []string) {
	if len(builtImages) <= 0 {
		return
	}
	if os.Getenv("DOCKER_SCAN_SUGGEST") == "false" {
		return
	}
	if scanAvailable() {
		commands := []string{}
		for _, image := range builtImages {
			commands = append(commands, fmt.Sprintf("docker scan %s", image))
		}
		allCommands := strings.Join(commands, ", ")
		fmt.Printf("Try scanning the image you have just built to identify vulnerabilities with Docker’s new security tool: %s\n", allCommands)
	}
}

func scanAvailable() bool {
	cli, err := command.NewDockerCli()
	if err != nil {
		return false
	}
	plugins, err := pluginmanager.ListPlugins(cli, nil)
	if err != nil {
		return false
	}
	for _, plugin := range plugins {
		if plugin.Name == "scan" {
			return true
		}
	}
	return false
}
