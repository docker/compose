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

var managementCommands = []string{"ecs", "scan"}

var commands = []string{}

func main() {
	fmt.Println("Walking through docker help to list commands...")
	getCommands()
	getCommands("compose")

	fmt.Printf(`
var managementCommands = []string{
	"help",
	"%s",
}

var commands = []string{
	"%s",
}
`, strings.Join(managementCommands, "\", \n\t\""), strings.Join(commands, "\", \n\t\""))
}

const (
	mgtCommandsSection = "Management Commands:"
	commandsSection    = "Commands:"
	aliasesSection     = "Aliases:"
)

func getCommands(execCommands ...string) {
	withHelp := append(execCommands, "--help")
	cmd := exec.Command("docker", withHelp...)
	output, err := cmd.Output()
	if err != nil {
		return
	}
	text := string(output)
	lines := strings.Split(text, "\n")
	section := ""
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, mgtCommandsSection) {
			section = mgtCommandsSection
			continue
		}
		if strings.HasPrefix(trimmedLine, commandsSection) || strings.HasPrefix(trimmedLine, "Available Commands:") {
			section = commandsSection
			if len(execCommands) > 0 {
				command := execCommands[len(execCommands)-1]
				managementCommands = append(managementCommands, command)
			}
			continue
		}
		if strings.HasPrefix(trimmedLine, aliasesSection) {
			section = aliasesSection
			continue
		}
		if trimmedLine == "" {
			section = ""
			continue
		}

		tokens := strings.Split(trimmedLine, " ")
		command := strings.Replace(tokens[0], "*", "", 1)
		switch section {
		case mgtCommandsSection:
			getCommands(append(execCommands, command)...)
		case commandsSection:
			if !utils.StringContains(commands, command) {
				commands = append(commands, command)
			}
			getCommands(append(execCommands, command)...)
		case aliasesSection:
			aliases := strings.Split(trimmedLine, ",")
			for _, alias := range aliases {
				trimmedAlias := strings.TrimSpace(alias)
				if !utils.StringContains(commands, trimmedAlias) {
					commands = append(commands, trimmedAlias)
				}
			}
		}
	}
}
