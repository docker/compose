package commands

import (
	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func AddFlags(o *cli.ProjectOptions, flags *pflag.FlagSet) {
	flags.StringArrayVarP(&o.ConfigPaths, "file", "f", nil, "Specify an alternate compose file")
	flags.StringVarP(&o.Name, "project-name", "n", "", "Specify an alternate project name (default: directory name)")
}

type ProjectFunc func(project *types.Project, args []string) error

// WithProject wrap a ProjectFunc into a cobra command
func WithProject(options *cli.ProjectOptions, f ProjectFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		project, err := cli.ProjectFromOptions(options)
		if err != nil {
			return err
		}
		return f(project, args)
	}
}
