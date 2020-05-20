package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	. "github.com/docker/api/tests/framework"
)

type E2eSuite struct {
	Suite
}

func (s *E2eSuite) TestContextHelp() {
	It("ensures context command includes azure-login and aci-create", func() {
		output := s.NewDockerCommand("context", "create", "--help").ExecOrDie()
		Expect(output).To(ContainSubstring("docker context create CONTEXT BACKEND [OPTIONS] [flags]"))
		Expect(output).To(ContainSubstring("--aci-location"))
		Expect(output).To(ContainSubstring("--aci-subscription-id"))
		Expect(output).To(ContainSubstring("--aci-resource-group"))
	})
}

func (s *E2eSuite) TestContextDefault() {
	It("should be initialized with default context", func() {
		s.NewDockerCommand("context", "use", "default").ExecOrDie()
		output := s.NewDockerCommand("context", "show").ExecOrDie()
		Expect(output).To(ContainSubstring("default"))
		output = s.NewCommand("docker", "context", "ls").ExecOrDie()
		Expect(output).To(Not(ContainSubstring("test-example")))
		Expect(output).To(ContainSubstring("default *"))
	})
}

func (s *E2eSuite) TestLegacy() {
	It("should list all legacy commands", func() {
		output := s.NewDockerCommand("--help").ExecOrDie()
		Expect(output).To(ContainSubstring("swarm"))
	})

	It("should execute legacy commands", func() {
		output, _ := s.NewDockerCommand("swarm", "join").Exec()
		Expect(output).To(ContainSubstring("\"docker swarm join\" requires exactly 1 argument."))
	})

	It("should run local container in less than 10 secs", func() {
		s.NewDockerCommand("pull", "hello-world").ExecOrDie()
		output := s.NewDockerCommand("run", "--rm", "hello-world").WithTimeout(time.NewTimer(10 * time.Second).C).ExecOrDie()
		Expect(output).To(ContainSubstring("Hello from Docker!"))
	})
}

func (s *E2eSuite) TestMockBackend() {
	It("creates a new test context to hardcoded example backend", func() {
		s.NewDockerCommand("context", "create", "test-example", "example").ExecOrDie()
		// Expect(output).To(ContainSubstring("test-example context acitest created"))
	})

	It("uses the test context", func() {
		currentContext := s.NewDockerCommand("context", "use", "test-example").ExecOrDie()
		Expect(currentContext).To(ContainSubstring("test-example"))
		output := s.NewDockerCommand("context", "ls").ExecOrDie()
		Expect(output).To(ContainSubstring("test-example *"))
		output = s.NewDockerCommand("context", "show").ExecOrDie()
		Expect(output).To(ContainSubstring("test-example"))
	})

	It("can run ps command", func() {
		output := s.NewDockerCommand("ps").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(3))
		Expect(lines[2]).To(ContainSubstring("1234                alpine"))
	})

	It("can run quiet ps command", func() {
		output := s.NewDockerCommand("ps", "-q").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(2))
		Expect(lines[0]).To(Equal("id"))
		Expect(lines[1]).To(Equal("1234"))
	})

	It("can run ps command with all ", func() {
		output := s.NewDockerCommand("ps", "-q", "--all").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(3))
		Expect(lines[0]).To(Equal("id"))
		Expect(lines[1]).To(Equal("1234"))
		Expect(lines[2]).To(Equal("stopped"))
	})

	It("can run 'run' command", func() {
		output := s.NewDockerCommand("run", "nginx", "-p", "80:80").ExecOrDie()
		Expect(output).To(ContainSubstring("Running container \"nginx\" with name"))
	})
}

func (s *E2eSuite) TestAPIServer() {
	_, err := exec.LookPath("yarn")
	if err != nil || os.Getenv("SKIP_NODE") != "" {
		s.T().Skip("skipping, yarn not installed")
	}
	It("can run 'serve' command", func() {
		cName := "test-example"
		s.NewDockerCommand("context", "create", cName, "example").ExecOrDie()

		sPath := fmt.Sprintf("unix:///%s/docker.sock", s.ConfigDir)
		server, err := serveAPI(s.ConfigDir, sPath)
		Expect(err).To(BeNil())
		defer killProcess(server)

		s.NewCommand("yarn", "install").WithinDirectory("../node-client").ExecOrDie()
		output := s.NewCommand("yarn", "run", "start", cName, sPath).WithinDirectory("../node-client").ExecOrDie()
		Expect(output).To(ContainSubstring("nginx"))
	})
}

func TestE2e(t *testing.T) {
	suite.Run(t, new(E2eSuite))
}

func killProcess(process *os.Process) {
	err := process.Kill()
	Expect(err).To(BeNil())
}

func serveAPI(configDir string, address string) (*os.Process, error) {
	cmd := exec.Command("../../bin/docker", "--config", configDir, "serve", "--address", address)
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
