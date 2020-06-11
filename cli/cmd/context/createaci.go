/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
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
