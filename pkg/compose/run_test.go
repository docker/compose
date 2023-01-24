package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"gotest.tools/v3/assert"
)

func TestServiceEnvFiles(t *testing.T) {

	t.Run("Verify service.EnvFile shouldn't modify", func(t *testing.T) {
		fooService := types.ServiceConfig{
			Name:    "foo",
			EnvFile: []string{},
		}

		project := types.Project{
			Name: "test-project",
			Services: types.Services{
				fooService,
			},
		}

		opts := api.RunOptions{
			ServiceEnvFiles: nil,
		}

		applyRunOptions(&project, &fooService, opts)

		assert.Assert(t, len(fooService.EnvFile) == 0)
	})

	t.Run("Verify appends ServiceEnvFiles", func(t *testing.T) {
		fooService := &types.ServiceConfig{
			Name:    "foo",
			EnvFile: []string{"./existing.env"},
		}

		project := types.Project{
			Name: "test-project",
			Services: types.Services{
				*fooService,
			},
		}

		opts := api.RunOptions{
			ServiceEnvFiles: []string{"./file.env"},
		}

		applyRunOptions(&project, fooService, opts)

		assert.Assert(t, len(fooService.EnvFile) == 2)
		assert.Assert(t, fooService.EnvFile[0] == "./existing.env")
		assert.Assert(t, fooService.EnvFile[1] == "./file.env")
	})
}
