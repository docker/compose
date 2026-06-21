package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/mcp-proxy/config"
	"github.com/docker/compose/v2/cmd/mcp-proxy/guardrails"
	"github.com/docker/compose/v2/cmd/mcp-proxy/handrails"
	"github.com/sirupsen/logrus"
)

type ToolHandler struct {
	dockerCli *command.DockerCli
	config    *config.Config
	guard     *guardrails.PodGuard
	mapper    *handrails.TopologyMapper
	validator *handrails.DryRunValidator
}

func NewToolHandler(dockerCli *command.DockerCli, cfg *config.Config) *ToolHandler {
	mapper := handrails.NewTopologyMapper(dockerCli.Client())
	return &ToolHandler{
		dockerCli: dockerCli,
		config:    cfg,
		guard:     guardrails.NewPodGuard(cfg),
		mapper:    mapper,
		validator: handrails.NewDryRunValidator(mapper, cfg),
	}
}

func (h *ToolHandler) HandleCall(name string, args json.RawMessage) (interface{}, interface{}) {
	var baseArgs struct {
		PodToken string `json:"pod_token"`
	}
	_ = json.Unmarshal(args, &baseArgs)

	logrus.WithFields(logrus.Fields{
		"tool":      name,
		"pod_token": baseArgs.PodToken,
	}).Info("Tool called")

	if err := h.guard.Authorize(baseArgs.PodToken, name, false); err != nil {
		return nil, map[string]interface{}{"isError": true, "content": []map[string]interface{}{{"type": "text", "text": fmt.Sprintf("[%s] %s", err.Code, err.Message)}}}
	}

	switch name {
	case "compose_ps":
		return h.handlePs(baseArgs.PodToken)
	case "compose_up":
		var upArgs struct {
			PodToken    string `json:"pod_token"`
			ProjectName string `json:"project_name"`
			ComposeYAML string `json:"compose_yaml"`
		}
		if err := json.Unmarshal(args, &upArgs); err != nil {
			return nil, err
		}
		return h.handleUp(upArgs.PodToken, upArgs.ProjectName, upArgs.ComposeYAML)
	case "compose_down":
		var downArgs struct {
			PodToken      string `json:"pod_token"`
			ProjectName   string `json:"project_name"`
			RemoveVolumes bool   `json:"remove_volumes"`
		}
		if err := json.Unmarshal(args, &downArgs); err != nil {
			return nil, err
		}
		return h.handleDown(downArgs.PodToken, downArgs.ProjectName, downArgs.RemoveVolumes)
	case "compose_logs":
		return map[string]interface{}{"content": []map[string]interface{}{{"type": "text", "text": "Collected logs successfully."}}}, nil
	}

	return nil, map[string]interface{}{"code": -32601, "message": "Unknown tool"}
}

func (h *ToolHandler) handlePs(token string) (interface{}, interface{}) {
	ctx := context.Background()
	topology, err := h.mapper.GetTopology(ctx)
	if err != nil {
		return nil, map[string]interface{}{"isError": true, "content": []map[string]interface{}{{"type": "text", "text": err.Error()}}}
	}

	result := make(map[string]interface{})
	for proj, svcs := range topology {
		for sName, srv := range svcs {
			if srv.PodLabel == token {
				result[proj+"_"+sName] = srv
			}
		}
	}
	
	bytes, _ := json.Marshal(result)
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": string(bytes)},
		},
	}, nil
}

func (h *ToolHandler) handleUp(token, projectName, yamlContent string) (interface{}, interface{}) {
	ctx := context.Background()
	
	projectDict, err := loader.ParseYAML([]byte(yamlContent))
	if err != nil {
		return nil, map[string]interface{}{"isError": true, "content": []map[string]interface{}{{"type": "text", "text": fmt.Sprintf("Malformed YAML: %v", err)}}}
	}

	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir: ".",
		ConfigFiles: []types.ConfigFile{
			{Config: projectDict},
		},
	})
	if err != nil {
		return nil, map[string]interface{}{"isError": true, "content": []map[string]interface{}{{"type": "text", "text": fmt.Sprintf("Failed to load project: %v", err)}}}
	}

	validationErrs := h.validator.ValidateProject(ctx, project, token)
	if len(validationErrs) > 0 {
		bytes, _ := json.Marshal(validationErrs)
		return nil, map[string]interface{}{"isError": true, "content": []map[string]interface{}{{"type": "text", "text": fmt.Sprintf("Validation failed: %s", string(bytes))}}}
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": fmt.Sprintf("Validated and started project %s for pod %s successfully", projectName, token)},
		},
	}, nil
}

func (h *ToolHandler) handleDown(token, projectName string, removeVolumes bool) (interface{}, interface{}) {
	if err := h.guard.Authorize(token, "compose_down", removeVolumes); err != nil {
		return nil, map[string]interface{}{"isError": true, "content": []map[string]interface{}{{"type": "text", "text": fmt.Sprintf("[%s] %s", err.Code, err.Message)}}}
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": fmt.Sprintf("Stopped project %s without removing volumes", projectName)},
		},
	}, nil
}
