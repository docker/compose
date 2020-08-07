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
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
	"github.com/docker/api/errdefs"
)

// Command returns the compose command with its child commands
func Command() *cobra.Command {
	command := &cobra.Command{
		Short: "Docker Compose",
		Use:   "compose",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return checkComposeSupport(cmd.Context())
		},
	}

	command.AddCommand(
		upCommand(),
		downCommand(),
		psCommand(),
		logsCommand(),
	)

	return command
}

func checkComposeSupport(ctx context.Context) error {
	c, err := client.New(ctx)
	if err == nil {
		composeService := c.ComposeService()
		if composeService == nil {
			return errors.New("compose not implemented in current context")
		}
		return nil
	}
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)
	cc, err := s.Get(currentContext)
	if err != nil {
		return err
	}
	switch cc.Type() {
	case store.AwsContextType:
		return errors.New("use 'docker ecs compose' on context type " + cc.Type())
	default:
		return errors.Wrapf(errdefs.ErrNotImplemented, "compose command not supported on context type %q", cc.Type())
	}
}
