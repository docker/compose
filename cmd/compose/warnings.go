package compose

import (
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
)

// warnIgnoredDeployAttributes emits informational warnings for deploy
// attributes that are ignored by Docker Compose when running in
// standalone (non-Swarm) mode.
//
// Note that some deploy attributes are still considered in standalone
// mode, most notably `replicas` and device reservations (e.g. GPU requests).
func warnIgnoredDeployAttributes(project *types.Project) {
	for _, svc := range project.Services {
		if svc.Deploy == nil {
			continue
		}

		ignored := collectIgnoredDeployAttributes(svc.Deploy)
		if len(ignored) == 0 {
			continue
		}

		sort.Strings(ignored)

		logrus.Warnf(
			"Service %q uses deploy attributes that are ignored by Docker Compose "+
				"in standalone (non-Swarm) mode. Some deploy attributes are still considered "+
				"(such as replicas and device reservations), but the following will be ignored: %s",
			svc.Name,
			strings.Join(ignored, ", "),
		)
	}
}

// collectIgnoredDeployAttributes returns deploy sub-attributes
// that are ignored by Docker Compose in standalone mode.
//
// This function intentionally focuses only on attributes that are
// consistently ignored, to avoid noisy or misleading warnings.
func collectIgnoredDeployAttributes(deploy *types.DeployConfig) []string {
	if deploy == nil {
		return nil
	}

	var ignored []string

	// Placement — ignored in standalone mode
	if len(deploy.Placement.Constraints) > 0 {
		ignored = append(ignored, "placement.constraints")
	}
	if len(deploy.Placement.Preferences) > 0 {
		ignored = append(ignored, "placement.preferences")
	}

	// Update / Rollback configuration — ignored
	if deploy.UpdateConfig != nil {
		ignored = append(ignored, "update_config")
	}
	if deploy.RollbackConfig != nil {
		ignored = append(ignored, "rollback_config")
	}

	// Endpoint mode — ignored
	if deploy.EndpointMode != "" {
		ignored = append(ignored, "endpoint_mode")
	}

	// Mode — ignored in standalone mode (even if set to "replicated")
	if deploy.Mode != "" {
		ignored = append(ignored, "mode")
	}

	// Intentionally NOT warned:
	// - deploy.replicas (supported)
	// - deploy.resources.reservations.devices (supported, e.g. GPU requests)
	// - deploy.restart_policy (may be used as fallback if service.restart is unset)
	// - cpu/memory limits (runtime-dependent and partially supported)
	// - deploy.labels (ignored but rarely used)

	return ignored
}
