package compose

import (
	_ "github.com/compose-spec/compose-go/types"
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
