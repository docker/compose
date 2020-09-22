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

package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

// EcsCommand is a placeholder to drive early users to the integrated form of ecs support instead of its early plugin form
func EcsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "ecs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("The ECS integration is now part of the CLI. Use `docker compose` with an ECS context.") // nolint
		},
	}

	return cmd
}
