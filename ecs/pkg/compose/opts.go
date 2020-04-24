package compose

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type ProjectOptions struct {
	ConfigPaths []string
	name        string
}

func (o *ProjectOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringArrayVarP(&o.ConfigPaths, "file", "f", nil, "Specify an alternate compose file")
	flags.StringVarP(&o.name, "project-name", "n", "", "Specify an alternate project name (default: directory name)")
}

type ProjectFunc func(project *Project, args []string) error

// WithProject wrap a ProjectFunc into a cobra command
func WithProject(options *ProjectOptions, f ProjectFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		project, err := ProjectFromOptions(options)
		if err != nil {
			return err
		}
		return f(project, args)
	}
}
