package compose

import (
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

type Project struct {
	types.Config
	projectDir string
	Name       string `yaml:"-" json:"-"`
}

func NewProject(config types.ConfigDetails, name string) (*Project, error) {
	model, err := loader.Load(config)
	if err != nil {
		return nil, err
	}

	p := Project{
		Config:     *model,
		projectDir: config.WorkingDir,
		Name:       name,
	}
	return &p, nil
}
