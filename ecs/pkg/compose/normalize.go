package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
)

// Normalize a compose-go model to move deprecated attributes to canonical position, and introduce implicit defaults
// FIXME move this to compose-go
func Normalize(model *types.Config) error {
	if len(model.Networks) == 0 {
		// Compose application model implies a default network if none is explicitly set.
		model.Networks["default"] = types.NetworkConfig{
			Name: "default",
		}
	}

	for i, s := range model.Services {
		if len(s.Networks) == 0 {
			// Service without explicit network attachment are implicitly exposed on default network
			s.Networks = map[string]*types.ServiceNetworkConfig{"default": nil}
		}

		if s.LogDriver != "" {
			logrus.Warn("`log_driver` is deprecated. Use the `logging` attribute")
			if s.Logging == nil {
				s.Logging = &types.LoggingConfig{}
			}
			if s.Logging.Driver == "" {
				s.Logging.Driver = s.LogDriver
			} else {
				return fmt.Errorf("can't use both 'log_driver' (deprecated) and 'logging.driver'")
			}
		}
		if len(s.LogOpt) != 0 {
			logrus.Warn("`log_opts` is deprecated. Use the `logging` attribute")
			if s.Logging == nil {
				s.Logging = &types.LoggingConfig{}
			}
			for k, v := range s.LogOpt {
				if _, ok := s.Logging.Options[k]; !ok {
					s.Logging.Options[k] = v
				} else {
					return fmt.Errorf("can't use both 'log_opt' (deprecated) and 'logging.options'")
				}
			}
		}
		model.Services[i] = s
	}
	return nil
}
