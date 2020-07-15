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

package context

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/docker/api/cli/mobycli"
	"github.com/docker/api/context/store"
)

type descriptionCreateOpts struct {
	description string
}

func createCommand() *cobra.Command {
	const longHelp = `Create a new context

Create docker engine context: 
$ docker context create CONTEXT [flags]

Create Azure Container Instances context:
$ docker context create aci CONTEXT [flags]
(see docker context create aci --help)

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

$ docker context create my-context --description "some description" --docker "host=tcp://myserver:2376,ca=~/ca-file,cert=~/cert-file,key=~/key-file"`

	cmd := &cobra.Command{
		Use:   "create CONTEXT",
		Short: "Create new context",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mobycli.ExecCmd(cmd)
		},
		Long: longHelp,
	}

	cmd.AddCommand(
		createAciCommand(),
		createAwsCommand(),
		createLocalCommand(),
		createExampleCommand(),
	)

	flags := cmd.Flags()
	flags.String("description", "", "Description of the context")
	flags.String(
		"default-stack-orchestrator", "",
		"Default orchestrator for stack operations to use with this context (swarm|kubernetes|all)")
	flags.StringToString("docker", nil, "set the docker endpoint")
	flags.StringToString("kubernetes", nil, "set the kubernetes endpoint")
	flags.String("from", "", "create context from a named context")

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
			return createDockerContext(cmd.Context(), args[0], store.LocalContextType, opts.description, store.LocalContext{})
		},
	}
	addDescriptionFlag(cmd, &opts.description)
	return cmd
}

func createExampleCommand() *cobra.Command {
	var opts descriptionCreateOpts
	cmd := &cobra.Command{
		Use:    "example CONTEXT",
		Short:  "Create a test context returning fixed output",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createDockerContext(cmd.Context(), args[0], store.ExampleContextType, opts.description, store.ExampleContext{})
		},
	}

	addDescriptionFlag(cmd, &opts.description)
	return cmd
}

func createDockerContext(ctx context.Context, name string, contextType string, description string, data interface{}) error {
	s := store.ContextStore(ctx)
	result := s.Create(
		name,
		contextType,
		description,
		data,
	)
	return result
}

func contextExists(ctx context.Context, name string) bool {
	s := store.ContextStore(ctx)
	return s.ContextExists(name)
}

func addDescriptionFlag(cmd *cobra.Command, descriptionOpt *string) {
	cmd.Flags().StringVar(descriptionOpt, "description", "", "Description of the context")
}
