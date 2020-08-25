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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/client"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/ecs"
	"github.com/docker/compose-cli/errdefs"
)

func init() {
	extraCommands = append(extraCommands, createEcsCommand)
	extraHelp = append(extraHelp, `
Create Amazon ECS context:
$ docker context create ecs CONTEXT [flags]
(see docker context create ecs --help)
`)
}

func createEcsCommand() *cobra.Command {
	var opts ecs.ContextParams
	cmd := &cobra.Command{
		Use:   "ecs CONTEXT [flags]",
		Short: "Create a context for Amazon ECS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateEcs(cmd.Context(), args[0], opts)
		},
	}

	addDescriptionFlag(cmd, &opts.Description)
	cmd.Flags().StringVar(&opts.Profile, "profile", "", "Profile")
	cmd.Flags().StringVar(&opts.Region, "region", "", "Region")
	cmd.Flags().StringVar(&opts.AwsID, "key-id", "", "AWS Access Key ID")
	cmd.Flags().StringVar(&opts.AwsSecret, "secret-key", "", "AWS Secret Access Key")
	return cmd
}

func runCreateEcs(ctx context.Context, contextName string, opts ecs.ContextParams) error {
	if contextExists(ctx, contextName) {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "context %q", contextName)
	}
	contextData, description, err := getEcsContextData(ctx, opts)
	if err != nil {
		return err
	}
	return createDockerContext(ctx, contextName, store.EcsContextType, description, contextData)

}

func getEcsContextData(ctx context.Context, opts ecs.ContextParams) (interface{}, string, error) {
	cs, err := client.GetCloudService(ctx, store.EcsContextType)
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot connect to AWS backend")
	}
	return cs.CreateContextData(ctx, opts)
}
