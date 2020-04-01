package compose

import (
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	registry "github.com/docker/cli/cli/registry/client"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/docker/client"
	"github.com/docker/helm-prototype/pkg/compose/internal/convert"
	"github.com/docker/helm-prototype/pkg/compose/internal/helm"
	utils "github.com/docker/helm-prototype/pkg/compose/internal/utils"
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

// Orchestrator is "kubernetes" or "swarm"
type Orchestrator string

const (
	// Kubernetes specifies to use kubernetes.
	Kubernetes Orchestrator = "kubernetes"
	// Swarm specifies to use Docker swarm.
	Swarm Orchestrator = "swarm"
)

type ProjectOptions struct {
	ConfigPaths []string
	Name        string
}

type Project struct {
	Config       *types.Config
	ProjectDir   string
	Name         string `yaml:"-" json:"-"`
	Orchestrator Orchestrator
}

func NewProject(config types.ConfigDetails, name string) (*Project, error) {
	model, err := loader.Load(config)
	if err != nil {
		return nil, err
	}

	p := Project{
		Config:     model,
		ProjectDir: config.WorkingDir,
		Name:       name,
	}
	return &p, nil
}

// projectFromOptions load a compose project based on command line options
func ProjectFromOptions(options *ProjectOptions) (*Project, error) {
	workingDir, configs, err := utils.GetConfigs(
		options.Name,
		options.ConfigPaths,
	)
	if err != nil {
		return nil, err
	}

	return NewProject(types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configs,
		Environment: utils.Environment(),
	}, options.Name)
}

func (p *Project) GenerateCharts(path string) error {
	objects, err := convert.MapToKubernetesObjects(p.Config, p.Name)
	if err != nil {
		return err
	}
	err = helm.Write(p.Name, objects, path)
	if err != nil {
		return err
	}
	return nil
}
func (p *Project) InstallCommand(options *ProjectOptions) error {
	return nil
}
