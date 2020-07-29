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

package compose

import (
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
	"github.com/docker/api/errdefs"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

// Command returns the compose command with its child commands
func Command() *cobra.Command {
	command := &cobra.Command{
		Short: "Docker Compose",
		Use:   "compose",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			currentContext := apicontext.CurrentContext(cmd.Context())
			s := store.ContextStore(cmd.Context())
			cc, err := s.Get(currentContext)
			if err != nil {
				return err
			}
			switch cc.Type() {
			case store.AciContextType:
				return nil
			case store.AwsContextType:
				return errors.New("use 'docker ecs compose' on context type " + cc.Type())
			default:
				return errors.Wrapf(errdefs.ErrNotImplemented, "compose command not supported on context type %q", cc.Type())
			}
		},
	}

	command.AddCommand(
		upCommand(),
		downCommand(),
	)

	return command
}
