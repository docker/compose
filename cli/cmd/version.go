/*
   Copyright 2020 Docker, Inc.

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
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/cli/cmd/mobyflags"
	"github.com/docker/compose-cli/cli/mobycli"
)

// VersionCommand command to display version
func VersionCommand(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the Docker version information",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVersion(cmd, version)
		},
	}
	// define flags for backward compatibility with com.docker.cli
	flags := cmd.Flags()
	flags.StringP("format", "f", "", "Format the output using the given Go template")
	flags.String("kubeconfig", "", "Kubernetes config file")
	mobyflags.AddMobyFlagsForRetrocompatibility(flags)

	return cmd
}

func runVersion(cmd *cobra.Command, version string) error {
	displayedVersion := strings.TrimPrefix(version, "v")
	versionResult, _ := mobycli.ExecSilent(cmd.Context())
	// we don't want to fail on error, there is an error if the engine is not available but it displays client version info
	// Still, technically the [] byte versionResult could be nil, just let the original command display what it has to display
	if versionResult == nil {
		mobycli.Exec()
		return nil
	}
	var s string = string(versionResult)
	fmt.Print(strings.Replace(s, "\n Version:", "\n Azure integration  "+displayedVersion+"\n Version:", 1))
	return nil
}
