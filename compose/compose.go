package compose

import (
	"os"

	"github.com/compose-spec/compose-go/types"
	internal "github.com/docker/helm-prototype/pkg/compose/internal"
	"github.com/docker/helm-prototype/pkg/compose/internal/helm"
	"github.com/docker/helm-prototype/pkg/compose/internal/kube"
)

var Settings = internal.GetDefault()

type ComposeProject struct {
	config     *types.Config
	helm       *helm.HelmActions
	ProjectDir string
	Name       string `yaml:"-" json:"-"`
}

type ComposeResult struct {
	Info       string
	Status     string
	Descriptin string
}

func Load(name string, configpaths []string) (*ComposeProject, error) {
	if name == "" {
		name = "docker-compose"
	}
	model, workingDir, err := internal.GetConfig(name, configpaths)
	if err != nil {
		return nil, err
	}
	return &ComposeProject{
		config:     model,
		helm:       helm.NewHelmActions(nil),
		ProjectDir: workingDir,
		Name:       name,
	}, nil
}

func (cp *ComposeProject) GenerateChart(dirname string) error {
	objects, err := kube.MapToKubernetesObjects(cp.config, cp.Name)
	if err != nil {
		return err
	}
	err = helm.Write(cp.Name, objects, dirname)
	if err != nil {
		return err
	}
	return nil
}

func (cp *ComposeProject) Install(name, path string) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path = cwd
	}
	return cp.helm.Install(name, path)
}

func (cp *ComposeProject) Uninstall(name string) error {
	return cp.helm.Uninstall(name)
}
