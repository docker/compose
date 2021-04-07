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
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/cli/cli"
	"github.com/docker/compose-cli/cli/formatter"
	"github.com/docker/compose-cli/cli/mobycli"
	"github.com/docker/compose-cli/internal"
)

const formatOpt = "format"

// VersionCommand command to display version
func VersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the Docker version information",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := runVersion(cmd)
			if err != nil {
				return cli.StatusError{StatusCode: 1, Status: err.Error()}
			}
			return nil
		},
	}
	// define flags for backward compatibility with com.docker.cli
	flags := cmd.Flags()
	flags.StringP(formatOpt, "f", "", "Format the output. Values: [pretty | json]. (Default: pretty)")
	flags.String("kubeconfig", "", "Kubernetes config file")

	return cmd
}

func runVersion(cmd *cobra.Command) error {
	var versionString string
	var err error
	format := strings.ToLower(strings.ReplaceAll(cmd.Flag(formatOpt).Value.String(), " ", ""))
	displayedVersion := strings.TrimPrefix(internal.Version, "v")
	// Replace is preferred in this case to keep the order.
	switch format {
	case formatter.PRETTY, "":
		versionString, err = getOutFromMoby(cmd, fixedPrettyArgs(os.Args[1:])...)
		versionString = strings.Replace(versionString,
			"\n Version:", "\n Cloud integration: "+displayedVersion+"\n Version:", 1)
	case formatter.JSON, formatter.TemplateLegacyJSON: // Try to catch full JSON formats
		versionString, err = getOutFromMoby(cmd, fixedJSONArgs(os.Args[1:])...)
		versionString = strings.Replace(versionString,
			`"Version":`, fmt.Sprintf(`"CloudIntegration":%q,"Version":`, displayedVersion), 1)
	default:
		versionString, err = getOutFromMoby(cmd)
	}

	fmt.Print(versionString)
	return err
}

func getOutFromMoby(cmd *cobra.Command, args ...string) (string, error) {
	versionResult, err := mobycli.ExecSilent(cmd.Context(), args...)
	// we don't want to fail on error, there is an error if the engine is not available but it displays client version info
	// Still, technically the [] byte versionResult could be nil, just let the original command display what it has to display
	if versionResult == nil {
		mobycli.Exec(cmd.Root())
		return "", nil
	}
	return string(versionResult), err
}

func fixedPrettyArgs(oArgs []string) []string {
	args := make([]string, 0)
	for i := 0; i < len(oArgs); i++ {
		if isFormatOpt(oArgs[i]) &&
			len(oArgs) > i &&
			(strings.ToLower(oArgs[i+1]) == formatter.PRETTY || oArgs[i+1] == "") {
			i++
			continue
		}
		args = append(args, oArgs[i])
	}
	return args
}

func fixedJSONArgs(oArgs []string) []string {
	args := make([]string, 0)
	for i := 0; i < len(oArgs); i++ {
		if isFormatOpt(oArgs[i]) &&
			len(oArgs) > i &&
			strings.ToLower(oArgs[i+1]) == formatter.JSON {
			args = append(args, oArgs[i], "{{json .}}")
			i++
			continue
		}
		args = append(args, oArgs[i])
	}
	return args
}

func isFormatOpt(o string) bool {
	return o == "--format" || o == "-f"
}
