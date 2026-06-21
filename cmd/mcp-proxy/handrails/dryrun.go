package handrails

import (
	"context"
	"fmt"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/cmd/mcp-proxy/config"
)

type ValidationError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type DryRunValidator struct {
	mapper *TopologyMapper
	config *config.Config
}

func NewDryRunValidator(mapper *TopologyMapper, cfg *config.Config) *DryRunValidator {
	return &DryRunValidator{mapper: mapper, config: cfg}
}

func (v *DryRunValidator) ValidateProject(ctx context.Context, project *types.Project, requesterToken string) []ValidationError {
	var errors []ValidationError

	topology, err := v.mapper.GetTopology(ctx)
	if err != nil {
		return []ValidationError{{Code: "ERR_STATE_UNREACHABLE", Message: "Cannot get current state", Suggestion: "Check Docker daemon"}}
	}

	// Gather reserved ports across other pods to ensure no conflict.
	otherReservedPorts := make(map[int]string)
	for podToken, podCfg := range v.config.Pods {
		if podToken != requesterToken {
			for _, p := range podCfg.ReservedPorts {
				otherReservedPorts[p] = podToken
			}
		}
	}

	// Ensure all existing containers holding ports are not conflicting.
	existingPorts := make(map[int]string)
	for _, services := range topology {
		for _, svc := range services {
			if svc.PodLabel != requesterToken {
				for _, pStr := range svc.Ports {
					p, _ := strconv.Atoi(pStr)
					if p != 0 {
						existingPorts[p] = svc.PodLabel
					}
				}
			}
		}
	}

	podCfg, ok := v.config.Pods[requesterToken]
	if !ok {
		return append(errors, ValidationError{
			Code:       "ERR_POD_UNKNOWN",
			Message:    fmt.Sprintf("Pod %s is not configured", requesterToken),
			Suggestion: "Check guard.json",
		})
	}

	memBytesLimit, _ := podCfg.MemoryBytes()

	for _, srv := range project.Services {
		// Rule: All services must have com.mcp.pod label matching the requester's token.
		podLabel, ok := srv.Labels["com.mcp.pod"]
		if !ok || podLabel != requesterToken {
			errors = append(errors, ValidationError{
				Code:       "ERR_MISSING_POD_LABEL",
				Message:    fmt.Sprintf("Service %s lacks matching com.mcp.pod label", srv.Name),
				Suggestion: fmt.Sprintf("Add com.mcp.pod: %s to labels", requesterToken),
			})
		}

		// Rule: No port conflicts
		for _, port := range srv.Ports {
			published, _ := strconv.Atoi(port.Published)
			if published != 0 {
				if owner, reserved := otherReservedPorts[published]; reserved {
					errors = append(errors, ValidationError{
						Code:       "ERR_PORT_CONFLICT",
						Message:    fmt.Sprintf("Port %d is reserved by %s Pod", published, owner),
						Suggestion: fmt.Sprintf("Map to an alternative port, e.g. %d", published+10000),
					})
				} else if owner, taken := existingPorts[published]; taken {
					errors = append(errors, ValidationError{
						Code:       "ERR_PORT_CONFLICT",
						Message:    fmt.Sprintf("Port %d is in use by %s Pod", published, owner),
						Suggestion: fmt.Sprintf("Map to an alternative port, e.g. %d", published+10000),
					})
				}
			}
		}

		// Rule: Resource ceilings
		if srv.Deploy != nil && srv.Deploy.Resources != nil && srv.Deploy.Resources.Limits != nil {
			if srv.Deploy.Resources.Limits.MemoryBytes != 0 && memBytesLimit > 0 {
				if int64(srv.Deploy.Resources.Limits.MemoryBytes) > memBytesLimit {
					errors = append(errors, ValidationError{
						Code:       "ERR_RESOURCE_LIMIT",
						Message:    fmt.Sprintf("Service %s exceeds pod memory limit of %s", srv.Name, podCfg.MemoryLimit),
						Suggestion: fmt.Sprintf("Lower memory limit to at most %s", podCfg.MemoryLimit),
					})
				}
			}
		} else {
			// Suggest adding resources
			if memBytesLimit > 0 {
				errors = append(errors, ValidationError{
					Code:       "ERR_MISSING_RESOURCES",
					Message:    fmt.Sprintf("Service %s has no resource limits defined", srv.Name),
					Suggestion: fmt.Sprintf("Define deploy.resources.limits to ensure you don't exceed %s", podCfg.MemoryLimit),
				})
			}
		}
	}

	return errors
}
