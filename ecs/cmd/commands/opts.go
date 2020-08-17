package commands

import (
	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/pflag"
)

type composeOptions struct {
	Name        string
	WorkingDir  string
	ConfigPaths []string
	Environment []string
}

func AddFlags(o *composeOptions, flags *pflag.FlagSet) {
	flags.StringArrayVarP(&o.ConfigPaths, "file", "f", nil, "Specify an alternate compose file")
	flags.StringVarP(&o.Name, "project-name", "n", "", "Specify an alternate project name (default: directory name)")
	flags.StringVarP(&o.WorkingDir, "workdir", "w", "", "Working directory")
	flags.StringSliceVarP(&o.Environment, "environment", "e", []string{}, "Environment variables")
}

func (o *composeOptions) toProjectOptions() (*cli.ProjectOptions, error) {
	return cli.NewProjectOptions(o.ConfigPaths,
		cli.WithOsEnv,
		cli.WithEnv(o.Environment),
		cli.WithWorkingDirectory(o.WorkingDir),
		cli.WithName(o.Name))
}
