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

	"github.com/docker/api/client"

	"github.com/spf13/cobra"

	"github.com/docker/api/context/store"
)

// AciCreateOpts Options for ACI context create
type AciCreateOpts struct {
	description       string
	aciLocation       string
	aciSubscriptionID string
	aciResourceGroup  string
}

func createCommand() *cobra.Command {
	var opts AciCreateOpts
	cmd := &cobra.Command{
		Use:   "create CONTEXT BACKEND [OPTIONS]",
		Short: "Create a context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), opts, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&opts.description, "description", "", "Description of the context")
	cmd.Flags().StringVar(&opts.aciLocation, "aci-location", "eastus", "Location")
	cmd.Flags().StringVar(&opts.aciSubscriptionID, "aci-subscription-id", "", "Location")
	cmd.Flags().StringVar(&opts.aciResourceGroup, "aci-resource-group", "", "Resource group")

	return cmd
}

func runCreate(ctx context.Context, opts AciCreateOpts, name string, contextType string) error {
	var description string
	var contextData interface{}

	switch contextType {
	case "aci":
		cs, err := client.GetCloudService(ctx, "aci")
		if err != nil {
			return errors.Wrap(err, "cannot connect to backend")
		}
		params := map[string]string{
			"aciSubscriptionId": opts.aciSubscriptionID,
			"aciResourceGroup":  opts.aciResourceGroup,
			"aciLocation":       opts.aciLocation,
			"description":       opts.description,
		}
		contextData, description, err = cs.CreateContextData(ctx, params)
		if err != nil {
			return errors.Wrap(err, "cannot create context")
		}
	default: // TODO: we need to implement different contexts for known backends
		description = opts.description
		contextData = store.ExampleContext{}
	}

	s := store.ContextStore(ctx)
	return s.Create(
		name,
		contextType,
		description,
		contextData,
	)
}
