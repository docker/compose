package compose

import (
	"os"

	internal "github.com/docker/helm-prototype/pkg/compose/internal"
)

type ProjectOptions struct {
	ConfigPaths []string
	Name        string
}

var Settings = internal.GetDefault()

type ComposeAPI struct {
	project *internal.Project
}

// projectFromOptions load a compose project based on command line options
func ProjectFromOptions(options *ProjectOptions) (*ComposeAPI, error) {
	if options == nil {
		options = &ProjectOptions{
			ConfigPaths: []string{},
			Name:        "docker-compose",
		}
	}

	if options.Name == "" {
		options.Name = "docker-compose"
	}

	project, err := internal.GetProject(options.Name, options.ConfigPaths)
	if err != nil {
		return nil, err
	}

	return &ComposeAPI{project: project}, nil
}

func (c *ComposeAPI) GenerateChart(dirname string) error {
	return c.project.ExportToCharts(dirname)
}

func (c *ComposeAPI) Install(name, path string) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path = cwd
	}
	return c.project.Install(name, path)
}

func (c *ComposeAPI) Uninstall(name string) error {
	return c.project.Uninstall(name)
}
