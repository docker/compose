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

	"github.com/docker/api/client"
	"github.com/docker/api/context/store"
)

type aciCreateOpts struct {
	description    string
	location       string
	subscriptionID string
	resourceGroup  string
}

func createAciCommand() *cobra.Command {
	var opts aciCreateOpts
	cmd := &cobra.Command{
		Use:   "aci CONTEXT [flags]",
		Short: "Create a context for Azure Container Instances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			contextData, description, err := getAciContextData(cmd.Context(), opts)
			if err != nil {
				return nil
			}
			return createDockerContext(cmd.Context(), args[0], store.AciContextType, description, contextData)
		},
	}

	addDescriptionFlag(cmd, &opts.description)
	cmd.Flags().StringVar(&opts.location, "location", "eastus", "Location")
	cmd.Flags().StringVar(&opts.subscriptionID, "subscription-id", "", "Location")
	cmd.Flags().StringVar(&opts.resourceGroup, "resource-group", "", "Resource group")

	return cmd
}

func getAciContextData(ctx context.Context, opts aciCreateOpts) (interface{}, string, error) {
	cs, err := client.GetCloudService(ctx, store.AciContextType)
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot connect to ACI backend")
	}
	return cs.CreateContextData(ctx, convertAciOpts(opts))
}

func convertAciOpts(opts aciCreateOpts) map[string]string {
	return map[string]string{
		"aciSubscriptionId": opts.subscriptionID,
		"aciResourceGroup":  opts.resourceGroup,
		"aciLocation":       opts.location,
		"description":       opts.description,
	}
}
