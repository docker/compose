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

package volume

import (
	"context"
	"fmt"

	"github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/cli/formatter"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/ecs"
	formatter2 "github.com/docker/compose-cli/formatter"
	"github.com/docker/compose-cli/progress"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
)

// Command manage volumes
func Command(ctype string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manages volumes",
	}

	cmd.AddCommand(
		createVolume(ctype),
		listVolume(),
		rmVolume(),
		inspectVolume(),
	)
	return cmd
}

func createVolume(ctype string) *cobra.Command {
	var usage string
	var short string
	switch ctype {
	case store.AciContextType:
		usage = "create --storage-account ACCOUNT VOLUME"
		short = "Creates an Azure file share to use as ACI volume."
	case store.EcsContextType:
		usage = "create [OPTIONS] VOLUME"
		short = "Creates an EFS filesystem to use as AWS volume."
	default:
		usage = "create [OPTIONS] VOLUME"
		short = "Creates a volume"
	}

	var opts interface{}
	cmd := &cobra.Command{
		Use:   usage,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := client.New(ctx)
			if err != nil {
				return err
			}
			result, err := progress.Run(ctx, func(ctx context.Context) (string, error) {
				volume, err := c.VolumeService().Create(ctx, args[0], opts)
				if err != nil {
					return "", err
				}
				return volume.ID, nil
			})
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	switch ctype {
	case store.AciContextType:
		aciOpts := aci.VolumeCreateOptions{}
		cmd.Flags().StringVar(&aciOpts.Account, "storage-account", "", "Storage account name")
		_ = cmd.MarkFlagRequired("storage-account")
		opts = &aciOpts
	case store.EcsContextType:
		ecsOpts := ecs.VolumeCreateOptions{}
		cmd.Flags().StringVar(&ecsOpts.KmsKeyID, "kms-key", "", "ID of the AWS KMS CMK to be used to protect the encrypted file system")
		cmd.Flags().StringVar(&ecsOpts.PerformanceMode, "performance-mode", "", "performance mode of the file system. (generalPurpose|maxIO)")
		cmd.Flags().Float64Var(&ecsOpts.ProvisionedThroughputInMibps, "provisioned-throughput", 0, "throughput in MiB/s (1-1024)")
		cmd.Flags().StringVar(&ecsOpts.ThroughputMode, "throughput-mode", "", "throughput mode (bursting|provisioned)")
		opts = &ecsOpts
	}
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
			formatter.SetMultiErrorFormat(errs)
			return errs.ErrorOrNil()
		},
	}
	return cmd
}

func inspectVolume() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect VOLUME [VOLUME...]",
		Short: "Inspect one or more volumes.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(cmd.Context())
			if err != nil {
				return err
			}
			v, err := c.VolumeService().Inspect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			outJSON, err := formatter2.ToStandardJSON(v)
			if err != nil {
				return err
			}
			fmt.Print(outJSON)
			return nil
		},
	}
	return cmd
}
