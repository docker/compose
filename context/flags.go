package context

import (
	"path/filepath"

	"github.com/docker/docker/pkg/homedir"
	"github.com/urfave/cli"
)

const (
	// ConfigFileName is the name of config file
	ConfigFileName = "config.json"
	configFileDir  = ".docker"
)

var (
	ConfigDir  string
	ConfigFlag = cli.StringFlag{
		Name:        "config",
		Usage:       "Location of client config files `DIRECTORY`",
		EnvVar:      "DOCKER_CONFIG",
		Value:       filepath.Join(homedir.Get(), configFileDir),
		Destination: &ConfigDir,
	}

	ContextName string
	ContextFlag = cli.StringFlag{
		Name:        "context, c",
		Usage:       "Name of the context `CONTEXT` to use to connect to the daemon (overrides DOCKER_HOST env var and default context set with \"docker context use\")",
		EnvVar:      "DOCKER_CONTEXT",
		Destination: &ContextName,
	}
)
