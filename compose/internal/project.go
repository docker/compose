package project

import (
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/helm-prototype/pkg/compose/internal/helm"
	"github.com/docker/helm-prototype/pkg/compose/internal/utils"

	"github.com/docker/helm-prototype/pkg/compose/internal/kube"
)

// Kind is "kubernetes" or "docker"
type Kind string

const (
	// Kubernetes specifies to use a kubernetes cluster.
	Kubernetes Kind = "kubernetes"
	// Docker specifies to use Docker engine.
	DockerEngine Kind = "docker"
)

type Engine struct {
	Namespace string

	Kind Kind

	Config string
	// Context is the name of the kubeconfig/docker context.
	Context string
	// Token used for authentication (kubernetes)
	Token string
	// Kubernetes API Server Endpoint for authentication
	APIServer string
}

func GetDefault() *Engine {
	return &Engine{Kind: Kubernetes}
}

type Project struct {
	Config     *types.Config
	Helm       *helm.HelmActions
	ProjectDir string
	Name       string `yaml:"-" json:"-"`
}

func NewProject(config types.ConfigDetails, name string) (*Project, error) {
	model, err := loader.Load(config)
	if err != nil {
		return nil, err
	}

	p := Project{
		Config:     model,
		Helm:       helm.NewHelmActions(nil),
		ProjectDir: config.WorkingDir,
		Name:       name,
	}
	return &p, nil
}

func GetProject(name string, configPaths []string) (*Project, error) {
	if name == "" {
		name = "docker-compose"
	}

	workingDir, configs, err := utils.GetConfigs(
		name,
		configPaths,
	)
	if err != nil {
		return nil, err
	}

	return NewProject(types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configs,
		Environment: utils.Environment(),
	}, name)

}

func (p *Project) ExportToCharts(path string) error {
	objects, err := kube.MapToKubernetesObjects(p.Config, p.Name)
	if err != nil {
		return err
	}
	err = helm.Write(p.Name, objects, path)
	if err != nil {
		return err
	}
	return nil
}

func (p *Project) Install(name, path string) error {
	return p.Helm.Install(name, path)
}

func (p *Project) Uninstall(name string) error {

	return p.Helm.Uninstall(name)
}
