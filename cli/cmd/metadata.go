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
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/internal"
)

// BackendMetadata backend metadata
// TODO import this from cli when merged & available in /docker/cli
type BackendMetadata struct {
	Name    string
	Version string
}

// MetadataCommand command to display version
func MetadataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "backend-metadata",
		Short:  "return CLI backend metadata",
		Args:   cobra.MaximumNArgs(0),
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			metadata := BackendMetadata{
				Name:    "Cloud integration",
				Version: internal.Version,
			}
			jsonMeta, err := json.Marshal(metadata)
			if err != nil {
				return err
			}
			fmt.Println(string(jsonMeta))
			return nil
		},
	}

	return cmd
}
