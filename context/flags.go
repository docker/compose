package context

import (
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
)

const (
	// ConfigFileName is the name of config file
	ConfigFileName = "config.json"
	configFileDir  = ".docker"
)

var (
	ConfigDir   string
	ContextName string
	ConfigFlag  = cli.StringFlag{
		Name:        "config",
		Usage:       "Location of client config files `DIRECTORY`",
		EnvVars:      []string{"DOCKER_CONFIG"},
		Value:       filepath.Join(home(), configFileDir),
		Destination: &ConfigDir,
	}

	ContextFlag = cli.StringFlag{
		Name:        "context",
		Aliases: 	 []string{"c"},
		Usage:       "Name of the context `CONTEXT` to use to connect to the daemon (overrides DOCKER_HOST env var and default context set with \"docker context use\")",
		EnvVars:      []string{"DOCKER_CONTEXT"},
		Destination: &ContextName,
	}
)

func home() string {
	home, _ := homedir.Dir()
	return home
}
