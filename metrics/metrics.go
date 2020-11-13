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

package metrics

import (
	"strings"

	"github.com/docker/compose-cli/utils"
)

// Track sends the tracking analytics to Docker Desktop
func Track(context string, args []string, status string) {
	command := GetCommand(args)
	if command != "" {
		c := NewClient()
		c.Send(Command{
			Command: command,
			Context: context,
			Source:  CLISource,
			Status:  status,
		})
	}
}

func isCommand(word string) bool {
	return utils.StringContains(commands, word) || isManagementCommand(word)
}

func isManagementCommand(word string) bool {
	return utils.StringContains(managementCommands, word)
}

func isCommandFlag(word string) bool {
	return utils.StringContains(commandFlags, word)
}

// GetCommand get the invoked command
func GetCommand(args []string) string {
	result := ""
	onlyFlags := false
	for _, arg := range args {
		if arg == "--help" {
			result = strings.TrimSpace(arg + " " + result)
			continue
		}
		if arg == "--" {
			break
		}
		if isCommandFlag(arg) || (!onlyFlags && isCommand(arg)) {
			result = strings.TrimSpace(result + " " + arg)
			if isCommand(arg) && !isManagementCommand(arg) {
				onlyFlags = true
			}
		}
	}
	return result
}
