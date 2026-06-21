package guardrails

import (
	"github.com/docker/compose/v2/cmd/mcp-proxy/config"
)

type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PodGuard struct {
	config        *config.Config
	allowedActions map[string]bool
}

func NewPodGuard(cfg *config.Config) *PodGuard {
	return &PodGuard{
		config: cfg,
		allowedActions: map[string]bool{
			"compose_up":   true,
			"compose_down": true,
			"compose_ps":   true,
			"compose_logs": true,
		},
	}
}

func (g *PodGuard) Authorize(token string, action string, removeVolumes bool) *AuthError {
	if _, ok := g.config.Pods[token]; !ok {
		return &AuthError{Code: "ERR_POD_NOT_FOUND", Message: "Requester pod token is not recognized"}
	}

	if !g.allowedActions[action] {
		return &AuthError{Code: "ERR_ACTION_FORBIDDEN", Message: "Action is not allowed"}
	}

	if action == "compose_down" && removeVolumes {
		return &AuthError{Code: "ERR_VOLUME_DESTRUCTION_BLOCKED", Message: "Cryptographic refusal: volume destruction is strictly prohibited"}
	}

	return nil
}
