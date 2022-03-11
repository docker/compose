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

package compatibility

import (
	"fmt"
	"os"

	"github.com/docker/compose/v2/cmd/compose"
)

func getCompletionCommands() []string {
	return []string{
		"__complete",
		"__completeNoDesc",
	}
}

func getBoolFlags() []string {
	return []string{
		"--debug", "-D",
		"--verbose",
		"--tls",
		"--tlsverify",
	}
}

func getStringFlags() []string {
	return []string{
		"--tlscacert",
		"--tlscert",
		"--tlskey",
		"--host", "-H",
		"--context",
		"--log-level",
	}
}

// Convert transforms standalone docker-compose args into CLI plugin compliant ones
func Convert(args []string) []string {
	var rootFlags []string
	command := []string{compose.PluginName}
	l := len(args)
	for i := 0; i < l; i++ {
		arg := args[i]
		if contains(getCompletionCommands(), arg) {
			command = append([]string{arg}, command...)
			continue
		}
		if len(arg) > 0 && arg[0] != '-' {
			// not a top-level flag anymore, keep the rest of the command unmodified
			if arg == compose.PluginName {
				i++
			}
			command = append(command, args[i:]...)
			break
		}

		switch arg {
		case "--verbose":
			arg = "--debug"
		case "-h":
			// docker cli has deprecated -h to avoid ambiguity with -H, while docker-compose still support it
			arg = "--help"
		case "--version", "-v":
			// redirect --version pseudo-command to actual command
			arg = "version"
		}

		if contains(getBoolFlags(), arg) {
			rootFlags = append(rootFlags, arg)
			continue
		}
		if contains(getStringFlags(), arg) {
			i++
			if i >= l {
				fmt.Fprintf(os.Stderr, "flag needs an argument: '%s'\n", arg)
				os.Exit(1)
			}
			rootFlags = append(rootFlags, arg, args[i])
			continue
		}
		command = append(command, arg)
	}
	return append(rootFlags, command...)
}

func contains(array []string, needle string) bool {
	for _, val := range array {
		if val == needle {
			return true
		}
	}
	return false
}
