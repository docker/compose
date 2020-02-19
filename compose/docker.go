package compose

import (
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	registry "github.com/docker/cli/cli/registry/client"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/docker/client"
)

var (
	Client         client.APIClient
	RegistryClient registry.RegistryClient
	ConfigFile     *configfile.ConfigFile
	Stdout         *streams.Out
)

func WithDockerCli(cli command.Cli) {
	Client = cli.Client()
	RegistryClient = cli.RegistryClient(false)
	ConfigFile = cli.ConfigFile()
	Stdout = cli.Out()
}
