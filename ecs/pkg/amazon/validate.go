package amazon

import (
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

// Validate check the compose model do not use unsupported features and inject sane defaults for ECS deployment
func (c *client) Validate(project *compose.Project) error {
	if len(project.Networks) == 0 {
		// Compose application model implies a default network if none is explicitly set.
		// FIXME move this to compose-go
		project.Networks["default"] = types.NetworkConfig{
			Name: "default",
		}
	}

	// Here we can check for incompatible attributes, inject sane defaults, etc
	return nil
}
