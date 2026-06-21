package mcp

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/mcp-proxy/config"
	"github.com/sirupsen/logrus"
)

type Server struct {
	dockerCli *command.DockerCli
	config    *config.Config
	tools     *ToolHandler
}

func NewServer(dockerCli *command.DockerCli, cfg *config.Config) *Server {
	return &Server{
		dockerCli: dockerCli,
		config:    cfg,
		tools:     NewToolHandler(dockerCli, cfg),
	}
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func (s *Server) Run(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		var req JSONRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			logrus.WithError(err).Warn("Failed to parse JSON-RPC request")
			continue
		}

		logrus.WithFields(logrus.Fields{
			"method": req.Method,
		}).Info("Intercepted MCP request")

		var res JSONRPCResponse
		res.JSONRPC = "2.0"
		res.ID = req.ID

		switch req.Method {
		case "initialize":
			res.Result = map[string]interface{}{"capabilities": map[string]interface{}{}, "serverInfo": map[string]string{"name": "compose-mcp-proxy", "version": "1.0.0"}}
		case "tools/list":
			res.Result = map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name": "compose_ps",
						"description": "List containers",
						"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"pod_token": map[string]interface{}{"type": "string"}}},
					},
					{
						"name": "compose_up",
						"description": "Create and start containers",
						"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"pod_token": map[string]interface{}{"type": "string"}, "project_name": map[string]interface{}{"type": "string"}, "compose_yaml": map[string]interface{}{"type": "string"}}},
					},
					{
						"name": "compose_down",
						"description": "Stop and remove containers",
						"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"pod_token": map[string]interface{}{"type": "string"}, "project_name": map[string]interface{}{"type": "string"}, "remove_volumes": map[string]interface{}{"type": "boolean"}}},
					},
					{
						"name": "compose_logs",
						"description": "View output from containers",
						"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"pod_token": map[string]interface{}{"type": "string"}, "project_name": map[string]interface{}{"type": "string"}}},
					},
				},
			}
		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err == nil {
				res.Result, res.Error = s.tools.HandleCall(params.Name, params.Arguments)
			} else {
				res.Error = map[string]interface{}{"code": -32602, "message": "Invalid params"}
			}
		default:
			res.Error = map[string]interface{}{"code": -32601, "message": "Method not found"}
		}

		if err := encoder.Encode(res); err != nil {
			logrus.WithError(err).Error("Failed to write response")
		}
	}
	return scanner.Err()
}
