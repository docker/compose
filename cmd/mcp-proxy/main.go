package main

import (
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/mcp-proxy/config"
	"github.com/docker/compose/v2/cmd/mcp-proxy/mcp"
	"github.com/sirupsen/logrus"
)

func main() {
	logFile, err := os.OpenFile("mcp-proxy.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(os.Stderr)
	}
	logrus.SetFormatter(&logrus.JSONFormatter{})

	guardConfigPath := os.Getenv("GUARD_CONFIG_PATH")
	if guardConfigPath == "" {
		guardConfigPath = `C:\ProgramData\compose-mcp-proxy\guard.json`
	}

	cfg, err := config.LoadGuardConfig(guardConfigPath)
	if err != nil {
		logrus.WithError(err).Warn("Failed to load guard configuration, proceeding with empty config")
		cfg = &config.Config{Pods: make(map[string]config.PodConfig)}
	}

	dockerCli, err := command.NewDockerCli()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize Docker CLI")
	}

	server := mcp.NewServer(dockerCli, cfg)
	
	logrus.Info("Starting MCP Proxy Server on stdio")
	if err := server.Run(os.Stdin, os.Stdout); err != nil {
		logrus.WithError(err).Fatal("Server encountered an error")
	}
}
