/*
   Copyright 2020 Docker Compose CLI authors

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
	"errors"
	"fmt"

	"github.com/docker/compose-cli/cmd/formatter"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
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
			return runRemove(args, opts.force)
		},
	}
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force removing current context")

	return cmd
}

func runRemove(args []string, force bool) error {
	currentContext := apicontext.Current()
	s := store.Instance()

	var errs *multierror.Error
	for _, contextName := range args {
		if currentContext == contextName {
			if force {
				if err := runUse("default"); err != nil {
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
	formatter.SetMultiErrorFormat(errs)
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
