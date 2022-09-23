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

package compose

import (
	"strings"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

// validArgsFn defines a completion func to be returned to fetch completion options
type validArgsFn func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

func noCompletion() validArgsFn {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{}, cobra.ShellCompDirectiveNoSpace
	}
}

func completeServiceNames(p *projectOptions) validArgsFn {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		project, err := p.toProject(nil)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var serviceNames []string
		for _, s := range project.ServiceNames() {
			if toComplete == "" || strings.HasPrefix(s, toComplete) {
				serviceNames = append(serviceNames, s)
			}
		}
		return serviceNames, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeProjectNames(backend api.Service) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		list, err := backend.List(cmd.Context(), api.ListOptions{
			All: true,
		})
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var values []string
		for _, stack := range list {
			if strings.HasPrefix(stack.Name, toComplete) {
				values = append(values, stack.Name)
			}
		}
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}
