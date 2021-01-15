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

package login

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/cli/cmd/mobyflags"
	"github.com/docker/compose-cli/cli/mobycli"
)

// Command returns the login command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [OPTIONS] [SERVER]",
		Short: "Log in to a Docker registry or cloud backend",
		Long:  "Log in to a Docker registry or cloud backend.\nIf no registry server is specified, the default is defined by the daemon.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runLogin,
	}
	// define flags for backward compatibility with com.docker.cli
	flags := cmd.Flags()
	flags.StringP("username", "u", "", "username")
	flags.StringP("password", "p", "", "password")
	flags.BoolP("password-stdin", "", false, "Take the password from stdin")
	mobyflags.AddMobyFlagsForRetrocompatibility(flags)

	cmd.AddCommand(AzureLoginCommand())
	return cmd
}

func runLogin(cmd *cobra.Command, args []string) error {
	mobycli.Exec(cmd.Root())
	return nil
}

func cloudLogin(cmd *cobra.Command, backendType string, params interface{}) error {
	ctx := cmd.Context()
	cs, err := client.GetCloudService(ctx, backendType)
	if err != nil {
		return errors.Wrap(errdefs.ErrLoginFailed, "cannot connect to backend")
	}
	err = cs.Login(ctx, params)
	if errors.Is(err, context.Canceled) {
		return errors.New("login canceled")
	}
	if err != nil {
		return err
	}
	fmt.Println("login succeeded")
	return nil
}
