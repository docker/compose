package compatibility

import (
	"testing"

	"github.com/docker/ecs-plugin/pkg/compose"
	"gotest.tools/v3/assert"
)

func load(t *testing.T, paths ...string) *compose.Project {
	options := compose.ProjectOptions{
		Name:        t.Name(),
		ConfigPaths: paths,
	}
	project, err := compose.ProjectFromOptions(&options)
	assert.NilError(t, err)
	return project
}
func TestInvalidNetworkMode(t *testing.T) {
	project := load(t, "../backend/testdata/invalid_network_mode.yaml")
	err := Check(project)
	assert.Error(t, err[0], "'network_mode' \"bridge\" is not supported")
}
