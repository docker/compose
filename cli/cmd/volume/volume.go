package volume

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

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/api/client"
)

// Command manage volumes
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manages volumes",
	}

	cmd.AddCommand(
		createVolume(),
		listVolume(),
	)
	return cmd
}

func createVolume() *cobra.Command {
	opts := aci.VolumeCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates an Azure file share to use as ACI volume.",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(cmd.Context())
			if err != nil {
				return err
			}
			id, err := c.VolumeService().Create(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Println(id)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Account, "storage-account", "", "Storage account name")
	cmd.Flags().StringVar(&opts.Fileshare, "fileshare", "", "Fileshare name")
	return cmd
}