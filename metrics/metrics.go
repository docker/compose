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

	flag "github.com/spf13/pflag"

	"github.com/docker/compose-cli/utils"
)

var managementCommands = []string{
	"app",
	"assemble",
	"builder",
	"buildx",
	"ecs",
	"ecs compose",
	"cluster",
	"compose",
	"config",
	"container",
	"context",
	// We add "context create" as a management command to be able to catch
	// calls to "context create aci"
	"context create",
	"help",
	"image",
	// Adding "login" as a management command so that the system can catch
	// commands like `docker login azure`
	"login",
	"manifest",
	"network",
	"node",
	"plugin",
	"registry",
	"secret",
	"service",
	"stack",
	"swarm",
	"system",
	"template",
	"trust",
	"volume",
}

// managementSubCommands holds a list of allowed subcommands of a management
// command. For example we want to send an event for "docker login azure" but
// we don't wat to send the name of the registry when the user does a
// "docker login my-registry", we only want to send "login"
var managementSubCommands = map[string][]string{
	"login": {
		"azure",
	},
	"context create": {
		"aci",
	},
}

const (
	scanCommand = "scan"
)

// Track sends the tracking analytics to Docker Desktop
func Track(context string, args []string, flags *flag.FlagSet, status string) {
	command := GetCommand(args, flags)
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

// GetCommand get the invoked command
func GetCommand(args []string, flags *flag.FlagSet) string {
	command := ""
	strippedArgs := stripFlags(args, flags)

	if len(strippedArgs) != 0 {
		command = strippedArgs[0]

		if command == scanCommand {
			return getScanCommand(args)
		}

		for {
			if utils.StringContains(managementCommands, command) {
				if sub := getSubCommand(command, strippedArgs[1:]); sub != "" {
					command += " " + sub
					strippedArgs = strippedArgs[1:]
					continue
				}
			}
			break
		}
	}

	return command
}

func getScanCommand(args []string) string {
	command := args[0]

	if utils.StringContains(args, "--auth") {
		return command + " auth"
	}

	if utils.StringContains(args, "--version") {
		return command + " version"
	}

	return command
}

func getSubCommand(command string, args []string) string {
	if len(args) == 0 {
		return ""
	}

	if val, ok := managementSubCommands[command]; ok {
		if utils.StringContains(val, args[0]) {
			return args[0]
		}
		return ""
	}

	if isArg(args[0]) {
		return args[0]
	}

	return ""
}

func stripFlags(args []string, flags *flag.FlagSet) []string {
	commands := []string{}

	for len(args) > 0 {
		s := args[0]
		args = args[1:]

		if s == "--" {
			return commands
		}

		if flagArg(s, flags) {
			if len(args) <= 1 {
				return commands
			}
			args = args[1:]
		}

		if isArg(s) {
			commands = append(commands, s)
		}
	}

	return commands
}

func flagArg(s string, flags *flag.FlagSet) bool {
	return strings.HasPrefix(s, "--") && !strings.Contains(s, "=") && !hasNoOptDefVal(s[2:], flags) ||
		strings.HasPrefix(s, "-") && !strings.Contains(s, "=") && len(s) == 2 && !shortHasNoOptDefVal(s[1:], flags)
}

func isArg(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-")
}

func hasNoOptDefVal(name string, fs *flag.FlagSet) bool {
	flag := fs.Lookup(name)
	if flag == nil {
		return false
	}

	return flag.NoOptDefVal != ""
}

func shortHasNoOptDefVal(name string, fs *flag.FlagSet) bool {
	flag := fs.ShorthandLookup(name[:1])
	if flag == nil {
		return false
	}

	return flag.NoOptDefVal != ""
}
