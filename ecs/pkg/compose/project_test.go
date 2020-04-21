package compose

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func Test_project_name(t *testing.T) {
	p, err := projectFromOptions(&ProjectOptions{
		name:        "my_project",
		ConfigPaths: []string{"testdata/simple/compose.yaml"},
	})
	assert.NilError(t, err)
	assert.Equal(t, p.Name, "my_project")

	p, err = projectFromOptions(&ProjectOptions{
		name:        "",
		ConfigPaths: []string{"testdata/simple/compose.yaml"},
	})
	assert.NilError(t, err)
	assert.Equal(t, p.Name, "simple")

	os.Setenv("COMPOSE_PROJECT_NAME", "my_project_from_env")
	p, err = projectFromOptions(&ProjectOptions{
		name:        "",
		ConfigPaths: []string{"testdata/simple/compose.yaml"},
	})
	assert.NilError(t, err)
	assert.Equal(t, p.Name, "my_project_from_env")
}

func Test_project_from_set_of_files(t *testing.T) {
	p, err := projectFromOptions(&ProjectOptions{
		name: "my_project",
		ConfigPaths: []string{
			"testdata/simple/compose.yaml",
			"testdata/simple/compose-with-overrides.yaml",
		},
	})
	assert.NilError(t, err)
	service, err := p.GetService("simple")
	assert.NilError(t, err)
	assert.Equal(t, service.Image, "haproxy")
}
