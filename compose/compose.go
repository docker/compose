package compose

import (
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	internal "github.com/docker/helm-prototype/pkg/compose/internal"
	"github.com/docker/helm-prototype/pkg/compose/internal/helm"
)

var Settings = internal.GetDefault()

type ComposeProject struct {
	config     *types.Config
	helm       *helm.HelmActions
	ProjectDir string
	Name       string `yaml:"-" json:"-"`
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
	if dirname == "" {
		dirname = cp.config.Filename
		if strings.Contains(dirname, ".") {
			splits := strings.SplitN(dirname, ".", 2)
			dirname = splits[0]
		}
	}
	name := filepath.Base(dirname)
	dirname = filepath.Dir(dirname)
	return internal.SaveChart(cp.config, name, dirname)
}

func (cp *ComposeProject) Install(name, path string) error {
	if path != "" {
		return cp.helm.InstallChartFromDir(name, path)
	}

	chart, err := internal.GetChartInMemory(cp.config, name)
	if err != nil {
		return err
	}
	return cp.helm.InstallChart(name, chart)
}

func (cp *ComposeProject) Uninstall(name string) error {
	return cp.helm.Uninstall(name)
}

func (cp *ComposeProject) List() (map[string]interface{}, error) {
	return cp.helm.ListReleases()
}
