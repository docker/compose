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

package volume

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/progress"
)

// ACICommand manage volumes
func ACICommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manages volumes",
	}

	cmd.AddCommand(
		createVolume(),
		listVolume(),
		rmVolume(),
	)
	return cmd
}

func createVolume() *cobra.Command {
	aciOpts := aci.VolumeCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates an Azure file share to use as ACI volume.",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := client.New(ctx)
			if err != nil {
				return err
			}
			err = progress.Run(ctx, func(ctx context.Context) error {
				if _, err := c.VolumeService().Create(ctx, aciOpts); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Println(aci.VolumeID(aciOpts.Account, aciOpts.Fileshare))
			return nil
		},
	}

	cmd.Flags().StringVar(&aciOpts.Account, "storage-account", "", "Storage account name")
	cmd.Flags().StringVar(&aciOpts.Fileshare, "fileshare", "", "Fileshare name")
	return cmd
}

func rmVolume() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm [OPTIONS] VOLUME [VOLUME...]",
		Short: "Remove one or more volumes.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(cmd.Context())
			if err != nil {
				return err
			}
			var errs *multierror.Error
			for _, id := range args {
				err = c.VolumeService().Delete(cmd.Context(), id, nil)
				if err != nil {
					errs = multierror.Append(errs, err)
					continue
				}
				fmt.Println(id)
			}
			if errs != nil {
				errs.ErrorFormat = formatErrors
			}
			return errs.ErrorOrNil()
		},
	}
	return cmd
}

func formatErrors(errs []error) string {
	messages := make([]string, len(errs))
	for i, err := range errs {
		messages[i] = "Error: " + err.Error()
	}
	return strings.Join(messages, "\n")
}
