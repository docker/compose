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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/config"
	"github.com/docker/compose-cli/api/context/store"
)

func useCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use CONTEXT",
		Short: "Set the default context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUse(args[0])
		},
	}
}

func runUse(name string) error {
	s := store.Instance()
	// Match behavior of existing CLI
	if name != store.DefaultContextName {
		if _, err := s.Get(name); err != nil {
			return err
		}
	}
	if err := config.WriteCurrentContext(config.Dir(), name); err != nil {
		return err
	}
	fmt.Println(name)
	return nil
}
