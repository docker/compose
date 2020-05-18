package amazon

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

// Validate check the compose model do not use unsupported features and inject sane defaults for ECS deployment
func Validate(project *compose.Project) error {
	if len(project.Networks) == 0 {
		// Compose application model implies a default network if none is explicitly set.
		// FIXME move this to compose-go
		project.Networks["default"] = types.NetworkConfig{
			Name: "default",
		}
	}

	for i, service := range project.Services {
		if len(service.Networks) == 0 {
			// Service without explicit network attachment are implicitly exposed on default network
			// FIXME move this to compose-go
			service.Networks = map[string]*types.ServiceNetworkConfig{"default": nil}
			project.Services[i] = service
		}

		if service.NetworkMode != "" && service.NetworkMode != "awsvpc" {
			return fmt.Errorf("ECS do not support NetworkMode %q", service.NetworkMode)
		}
	}

	// Here we can check for incompatible attributes, inject sane defaults, etc
	return nil
}
