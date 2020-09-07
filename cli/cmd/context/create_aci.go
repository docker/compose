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

	"github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/errdefs"
)

func init() {
	extraCommands = append(extraCommands, createAciCommand)
	extraHelp = append(extraHelp, `
Create Azure Container Instances context:
$ docker context create aci CONTEXT [flags]
(see docker context create aci --help)
`)
}

func createAciCommand() *cobra.Command {
	var opts aci.ContextParams
	cmd := &cobra.Command{
		Use:   "aci CONTEXT [flags]",
		Short: "Create a context for Azure Container Instances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateAci(cmd.Context(), args[0], opts)
		},
	}

	addDescriptionFlag(cmd, &opts.Description)
	cmd.Flags().StringVar(&opts.Location, "location", "eastus", "Location")
	cmd.Flags().StringVar(&opts.SubscriptionID, "subscription-id", "", "Location")
	cmd.Flags().StringVar(&opts.ResourceGroup, "resource-group", "", "Resource group")

	return cmd
}

func runCreateAci(ctx context.Context, contextName string, opts aci.ContextParams) error {
	if contextExists(ctx, contextName) {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "context %s", contextName)
	}
	contextData, description, err := getAciContextData(ctx, opts)
	if err != nil {
		return err
	}
	return createDockerContext(ctx, contextName, store.AciContextType, description, contextData)

}

func getAciContextData(ctx context.Context, opts aci.ContextParams) (interface{}, string, error) {
	cs, err := client.GetCloudService(ctx, store.AciContextType)
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot connect to ACI backend")
	}
	return cs.CreateContextData(ctx, opts)
}
