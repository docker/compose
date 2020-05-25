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

	"github.com/spf13/cobra"

	"github.com/docker/api/context/store"
)

type createOpts struct {
	description       string
	aciLocation       string
	aciSubscriptionID string
	aciResourceGroup  string
}

func createCommand() *cobra.Command {
	var opts createOpts
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

func runCreate(ctx context.Context, opts createOpts, name string, contextType string) error {
	switch contextType {
	case "aci":
		return createACIContext(ctx, name, opts)
	default:
		s := store.ContextStore(ctx)
		// TODO: we need to implement different contexts for known backends
		return s.Create(name, contextType, opts.description, store.ExampleContext{})
	}
}
