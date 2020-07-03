/*
   Copyright 2020 Docker, Inc.

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
)

var managementCommands = []string{
	"app",
	"assemble",
	"builder",
	"buildx",
	"ecs",
	"cluster",
	"compose",
	"config",
	"container",
	"context",
	"help",
	"image",
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

const (
	scanCommand = "scan"
)

// Track sends the tracking analytics to Docker Desktop
func Track(context string, args []string, flags *flag.FlagSet) {
	wasIn := make(chan bool)

	// Fire and forget, we don't want to slow down the user waiting for DD
	// metrics endpoint to respond. We could lose some events but that's ok.
	go func() {
		defer func() {
			_ = recover()
		}()

		wasIn <- true

		command := getCommand(args, flags)
		if command != "" {
			c := NewClient()
			c.Send(Command{
				Command: command,
				Context: context,
			})
		}
	}()
	<-wasIn
}

func getCommand(args []string, flags *flag.FlagSet) string {
	command := ""
	strippedArgs := stripFlags(args, flags)

	if len(strippedArgs) != 0 {
		command = strippedArgs[0]

		if command == scanCommand {
			return getScanCommand(args)
		}

		for {
			currentCommand := strippedArgs[0]
			if contains(managementCommands, currentCommand) {
				if sub := getSubCommand(strippedArgs[1:]); sub != "" {
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

	if contains(args, "--auth") {
		return command + " auth"
	}

	if contains(args, "--version") {
		return command + " version"
	}

	return command
}

func getSubCommand(args []string) string {
	if len(args) != 0 && isArg(args[0]) {
		return args[0]
	}
	return ""
}

func contains(array []string, needle string) bool {
	for _, val := range array {
		if val == needle {
			return true
		}
	}
	return false
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
