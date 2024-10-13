package compose

import (
	"fmt"
	"os/exec"
	"strings"
)

func killProcessesUsingExposedPorts(project *types.Project) error {
	for _, service := range project.Services {
		for _, port := range service.Ports {
			exposedPort := port.Published
			if exposedPort != "" {
				err := killProcessOnPort(exposedPort)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func killProcessOnPort(port string) error {
	cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf(":%s", port))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to find process using port %s: %v", port, err)
	}

	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return nil
	}

	killCmd := exec.Command("kill", "-9", pid)
	err = killCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to kill process %s on port %s: %v", pid, port, err)
	}

	fmt.Printf("Killed process %s using port %s\n", pid, port)
	return nil
}
