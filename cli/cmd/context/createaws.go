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

	"github.com/docker/api/amazon"
	"github.com/docker/api/client"
	"github.com/docker/api/context/store"
	"github.com/docker/api/errdefs"
)

func createAwsCommand() *cobra.Command {
	var opts amazon.ContextParams
	cmd := &cobra.Command{
		Use:   "aws CONTEXT [flags]",
		Short: "Create a context for Amazon ECS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateAws(cmd.Context(), args[0], opts)
		},
	}

	addDescriptionFlag(cmd, &opts.Description)
	cmd.Flags().StringVar(&opts.Profile, "profile", "", "AWS Profile")
	cmd.Flags().StringVar(&opts.Region, "region", "", "AWS region")
	cmd.Flags().StringVar(&opts.AwsID, "key-id", "", "AWS Access Key ID")
	cmd.Flags().StringVar(&opts.AwsSecret, "secret-key", "", "AWS Secret Access Key")
	return cmd
}

func runCreateAws(ctx context.Context, contextName string, opts amazon.ContextParams) error {
	if contextExists(ctx, contextName) {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "context %s", contextName)
	}
	contextData, description, err := getAwsContextData(ctx, opts)
	if err != nil {
		return err
	}
	return createDockerContext(ctx, contextName, store.AwsContextType, description, contextData)

}

func getAwsContextData(ctx context.Context, opts amazon.ContextParams) (interface{}, string, error) {
	cs, err := client.GetCloudService(ctx, store.AwsContextType)
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot connect to AWS backend")
	}
	return cs.CreateContextData(ctx, opts)
}
