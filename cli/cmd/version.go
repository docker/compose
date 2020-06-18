package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/api/cli/mobycli"
)

const cliVersion = "0.1.1"

// VersionCommand command to display version
func VersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the Docker version information",
		Args:  cobra.MaximumNArgs(0),
		RunE:  runVersion,
	}
	// define flags for backward compatibility with com.docker.cli
	flags := cmd.Flags()
	flags.String("format", "", "Format the output using the given Go template")
	flags.String("kubeconfig", "", "Kubernetes config file")

	return cmd
}

func runVersion(cmd *cobra.Command, args []string) error {
	versionResult, _ := mobycli.ExecSilent(cmd.Context())
	// we don't want to fail on error, there is an error if the engine is not available but it displays client version info
	// Still, technically the [] byte versionResult could be nil, just let the original command display what it has to display
	if versionResult == nil {
		return mobycli.ExecCmd(cmd)
	}
	var s string = string(versionResult)
	fmt.Print(strings.Replace(s, "\n Version:", "\n Azure integration  "+cliVersion+"\n Version:", 1))
	return nil
}
