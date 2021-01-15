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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/mobycli"
	"github.com/docker/compose-cli/errdefs"
)

func updateCommand() *cobra.Command {

	longHelp := `Update a context

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

$ docker context update my-context --description "some description" --docker "host=tcp://myserver:2376,ca=~/ca-file,cert=~/cert-file,key=~/key-file"`

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, args[0])
		},
		Long: longHelp,
	}
	flags := cmd.Flags()
	flags.String("description", "", "Description of the context")
	flags.String(
		"default-stack-orchestrator", "",
		"Default orchestrator for stack operations to use with this context (swarm|kubernetes|all)")
	flags.StringToString("docker", nil, "Set the docker endpoint")
	flags.StringToString("kubernetes", nil, "Set the kubernetes endpoint")

	return cmd
}

func runUpdate(cmd *cobra.Command, name string) error {
	s := store.ContextStore(cmd.Context())
	dockerContext, err := s.Get(name)
	if err == nil && dockerContext != nil {
		if dockerContext.Type() != store.DefaultContextType {
			return errors.Wrapf(errdefs.ErrNotImplemented, "context update for context type %q not supported", dockerContext.Type())
		}
	}

	mobycli.Exec(cmd.Root())
	return nil
}
