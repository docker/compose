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

package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/docker/compose-cli/utils"
)

var managementCommands = []string{"ecs", "assemble", "registry", "template", "cluster"}

var commands = []string{}

func main() {
	getCommands()
	getCommands("login")
	getCommands("context", "create")
	getCommands("compose")

	fmt.Printf(`
var managementCommands = []string{
	"%s",
}

var commands = []string{
	"%s",
}
`, strings.Join(managementCommands, "\", \n\t\""), strings.Join(commands, "\", \n\t\""))
}

func getCommands(execCommands ...string) {
	if len(execCommands) > 0 {
		managementCommands = append(managementCommands, execCommands[len(execCommands)-1])
	}
	withHelp := append(execCommands, "--help")
	cmd := exec.Command("docker", withHelp...)
	output, err := cmd.Output()
	if err != nil {
		return
	}
	text := string(output)
	lines := strings.Split(text, "\n")
	mgtCommandsStarted := false
	commandsStarted := false
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "Management Commands:") {
			mgtCommandsStarted = true
			continue
		}
		if strings.HasPrefix(trimmedLine, "Commands:") || strings.HasPrefix(trimmedLine, "Available Commands:") {
			mgtCommandsStarted = false
			commandsStarted = true
			continue
		}
		if trimmedLine == "" {
			mgtCommandsStarted = false
			commandsStarted = false
			continue
		}
		tokens := strings.Split(trimmedLine, " ")
		command := strings.Replace(tokens[0], "*", "", 1)
		if mgtCommandsStarted {
			getCommands(append(execCommands, command)...)
		}
		if commandsStarted {
			if !utils.StringContains(commands, command) {
				commands = append(commands, command)
			}
		}
	}
}
