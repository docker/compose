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
	"errors"
	"fmt"

	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
	"github.com/docker/api/multierror"
	"github.com/spf13/cobra"
)

type removeOpts struct {
	force bool
}

func removeCommand() *cobra.Command {
	var opts removeOpts
	cmd := &cobra.Command{
		Use:     "rm CONTEXT [CONTEXT...]",
		Short:   "Remove one or more contexts",
		Aliases: []string{"remove"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd.Context(), args, opts.force)
		},
	}
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "force removing current context")

	return cmd
}

func runRemove(ctx context.Context, args []string, force bool) error {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	var errs *multierror.Error
	for _, contextName := range args {
		if currentContext == contextName {
			if force {
				err := runUse(ctx, "default")
				if err != nil {
					errs = multierror.Append(errs, errors.New("cannot delete current context"))
				} else {
					errs = removeContext(s, contextName, errs)
				}
			} else {
				errs = multierror.Append(errs, errors.New("cannot delete current context"))
			}
		} else {
			errs = removeContext(s, contextName, errs)
		}
	}
	return errs.ErrorOrNil()
}

func removeContext(s store.Store, n string, errs *multierror.Error) *multierror.Error {
	if err := s.Remove(n); err != nil {
		errs = multierror.Append(errs, err)
	} else {
		fmt.Println(n)
	}
	return errs
}
