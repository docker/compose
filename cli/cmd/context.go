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

	"github.com/docker/api/context/store"
	"github.com/spf13/cobra"
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
	)

	return cmd
}

type createOpts struct {
	description string
}

func createCommand() *cobra.Command {
	var opts createOpts
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), opts, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&opts.description, "description", "", "Description of the context")

	return cmd
}

func runCreate(ctx context.Context, opts createOpts, name string, contextType string) error {
	s := store.ContextStore(ctx)
	return s.Create(name, store.TypeContext{
		Type:        contextType,
		Description: opts.description,
	}, map[string]interface{}{
		// If we don't set anything here the main docker cli
		// doesn't know how to read the context any more
		"docker": CliContext{},
	})
}
