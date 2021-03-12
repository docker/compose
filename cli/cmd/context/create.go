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

package context

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/mobycli"
)

type descriptionCreateOpts struct {
	description string
}

var extraCommands []func() *cobra.Command
var extraHelp []string

func createCommand() *cobra.Command {
	help := strings.Join(extraHelp, "\n")

	longHelp := fmt.Sprintf(`Create a new context

Create docker engine context:
$ docker context create CONTEXT [flags]

%s

Docker endpoint config:

NAME                DESCRIPTION
from                Copy named context's Docker endpoint configuration
host                Docker endpoint on which to connect
ca                  Trust certs signed only by this CA
cert                Path to TLS certificate file
key                 Path to TLS key file
skip-tls-verify     Skip TLS certificate validation

Kubernetes endpoint config:

NAME                 DESCRIPTION
from                 Copy named context's Kubernetes endpoint configuration
config-file          Path to a Kubernetes config file
context-override     Overrides the context set in the kubernetes config file
namespace-override   Overrides the namespace set in the kubernetes config file

Example:

$ docker context create my-context --description "some description" --docker "host=tcp://myserver:2376,ca=~/ca-file,cert=~/cert-file,key=~/key-file"`, help)

	cmd := &cobra.Command{
		Use:   "create CONTEXT",
		Short: "Create new context",
		RunE: func(cmd *cobra.Command, args []string) error {
			mobycli.Exec(cmd.Root())
			return nil
		},
		Long: longHelp,
	}

	cmd.AddCommand(
		createLocalCommand(),
	)
	for _, command := range extraCommands {
		cmd.AddCommand(command())
	}

	flags := cmd.Flags()
	flags.String("description", "", "Description of the context")
	flags.String(
		"default-stack-orchestrator", "",
		"Default orchestrator for stack operations to use with this context (swarm|kubernetes|all)")
	flags.StringToString("docker", nil, "Set the docker endpoint")
	flags.StringToString("kubernetes", nil, "Set the kubernetes endpoint")
	flags.String("from", "", "Create context from a named context")

	return cmd
}

func createLocalCommand() *cobra.Command {
	var opts descriptionCreateOpts
	cmd := &cobra.Command{
		Use:    "local CONTEXT",
		Short:  "Create a context for accessing local engine",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createDockerContext(args[0], store.LocalContextType, opts.description, store.LocalContext{})
		},
	}
	addDescriptionFlag(cmd, &opts.description)
	return cmd
}

func createDockerContext(name string, contextType string, description string, data interface{}) error {
	s := store.Instance()
	result := s.Create(
		name,
		contextType,
		description,
		data,
	)
	fmt.Printf("Successfully created %s context %q\n", contextType, name)
	return result
}

func contextExists(name string) bool {
	s := store.Instance()
	return s.ContextExists(name)
}

func addDescriptionFlag(cmd *cobra.Command, descriptionOpt *string) {
	cmd.Flags().StringVar(descriptionOpt, "description", "", "Description of the context")
}
