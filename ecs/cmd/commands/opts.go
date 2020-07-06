package commands

import (
	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/pflag"
)

func AddFlags(o *cli.ProjectOptions, flags *pflag.FlagSet) {
	flags.StringArrayVarP(&o.ConfigPaths, "file", "f", nil, "Specify an alternate compose file")
	flags.StringVarP(&o.Name, "project-name", "n", "", "Specify an alternate project name (default: directory name)")
}
