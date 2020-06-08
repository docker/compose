/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"gotest.tools/golden"

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
		output := s.NewDockerCommand("context", "show").ExecOrDie()
		Expect(output).To(ContainSubstring("default"))
		output = s.NewCommand("docker", "context", "ls").ExecOrDie()
		golden.Assert(s.T(), output, "ls-out-default.golden")
	})
}

func (s *E2eSuite) TestContextLegacy() {
	It("should inspect default", func() {
		output := s.NewDockerCommand("context", "inspect", "default").ExecOrDie()
		Expect(output).To(ContainSubstring(`"Name": "default"`))
	})
}

func (s *E2eSuite) TestClassicLoginWithparameters() {
	output, err := s.NewDockerCommand("login", "-u", "nouser", "-p", "wrongpasword").Exec()
	Expect(output).To(ContainSubstring("Get https://registry-1.docker.io/v2/: unauthorized: incorrect username or password"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestClassicLogin() {
	output, err := s.NewDockerCommand("login", "someregistry.docker.io").Exec()
	Expect(output).To(ContainSubstring("Cannot perform an interactive login from a non TTY device"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestCloudLogin() {
	output, err := s.NewDockerCommand("login", "mycloudbackend").Exec()
	Expect(output).To(ContainSubstring("unknown backend type for cloud login: mycloudbackend"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestSetupError() {
	It("should display an error if cannot shell out to docker-classic", func() {
		err := os.Setenv("PATH", s.BinDir)
		Expect(err).To(BeNil())
		err = os.Remove(filepath.Join(s.BinDir, "docker-classic"))
		Expect(err).To(BeNil())
		output, err := s.NewDockerCommand("ps").Exec()
		Expect(output).To(ContainSubstring("docker-classic"))
		Expect(output).To(ContainSubstring("not found"))
		Expect(err).NotTo(BeNil())
	})
}

func (s *E2eSuite) TestKillChildOnCancel() {
	It("should kill docker-classic if parent command is cancelled", func() {
		out := s.ListProcessesCommand().ExecOrDie()
		Expect(out).NotTo(ContainSubstring("docker-classic"))

		dir := s.ConfigDir
		Expect(ioutil.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(`FROM alpine:3.10
RUN sleep 100`), 0644)).To(Succeed())
		shutdown := make(chan time.Time)
		errs := make(chan error)
		ctx := s.NewDockerCommand("build", "--no-cache", "-t", "test-sleep-image", ".").WithinDirectory(dir).WithTimeout(shutdown)
		go func() {
			_, err := ctx.Exec()
			errs <- err
		}()
		err := WaitFor(time.Second, 10*time.Second, errs, func() bool {
			out := s.ListProcessesCommand().ExecOrDie()
			return strings.Contains(out, "docker-classic")
		})
		Expect(err).NotTo(HaveOccurred())
		log.Println("Killing docker process")

		close(shutdown)
		err = WaitFor(time.Second, 12*time.Second, nil, func() bool {
			out := s.ListProcessesCommand().ExecOrDie()
			return !strings.Contains(out, "docker-classic")
		})
		Expect(err).NotTo(HaveOccurred())
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

func (s *E2eSuite) TestLeaveLegacyErrorMessagesUnchanged() {
	output, err := s.NewDockerCommand("foo").Exec()
	golden.Assert(s.T(), output, "unknown-foo-command.golden")
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestDisplayFriendlyErrorMessageForLegacyCommands() {
	s.NewDockerCommand("context", "create", "test-example", "example").ExecOrDie()
	output, err := s.NewDockerCommand("--context", "test-example", "images").Exec()
	Expect(output).To(Equal("Command \"images\" not available in current context (test-example), you can use the \"default\" context to run this command\n"))
	Expect(err).NotTo(BeNil())
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
		golden.Assert(s.T(), output, "ls-out-test-example.golden")
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
