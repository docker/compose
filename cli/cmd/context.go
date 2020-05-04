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

package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/docker/api/context/store"
)

type CliContext struct {
}

func ContextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage contexts",
	}

	cmd.AddCommand(
		createCommand(),
		listCommand(),
	)

	return cmd
}

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

func listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context())
		},
	}
	return cmd
}

func runCreate(ctx context.Context, opts createOpts, name string, contextType string) error {
	switch contextType {
	case "aci":
		return createACIContext(ctx, name, opts)
	default:
		s := store.ContextStore(ctx)
		return s.Create(name, store.TypedContext{
			Type:        contextType,
			Description: opts.description,
		})
	}
}

func createACIContext(ctx context.Context, name string, opts createOpts) error {
	s := store.ContextStore(ctx)
	return s.Create(name, store.TypedContext{
		Type:        "aci",
		Description: opts.description,
		Data: store.AciContext{
			SubscriptionID: opts.aciSubscriptionID,
			Location:       opts.aciLocation,
			ResourceGroup:  opts.aciResourceGroup,
		},
	})
}

func runList(ctx context.Context) error {
	s := store.ContextStore(ctx)
	contexts, err := s.List()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tTYPE")
	format := "%s\t%s\t%s\n"

	for _, c := range contexts {
		fmt.Fprintf(w, format, c.Name, c.Metadata.Description, c.Metadata.Type)
	}

	return w.Flush()
}
