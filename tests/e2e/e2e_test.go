/*
   Copyright 2020 Docker, Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"os"
	"path/filepath"
	"runtime"
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
	output := s.NewDockerCommand("context", "create", "aci", "--help").ExecOrDie()
	Expect(output).To(ContainSubstring("docker context create aci CONTEXT [flags]"))
	Expect(output).To(ContainSubstring("--location"))
	Expect(output).To(ContainSubstring("--subscription-id"))
	Expect(output).To(ContainSubstring("--resource-group"))
}

func (s *E2eSuite) TestListAndShowDefaultContext() {
	output := s.NewDockerCommand("context", "show").ExecOrDie()
	Expect(output).To(ContainSubstring("default"))
	output = s.NewCommand("docker", "context", "ls").ExecOrDie()
	golden.Assert(s.T(), output, GoldenFile("ls-out-default"))
}

func (s *E2eSuite) TestCreateDockerContextAndListIt() {
	s.NewDockerCommand("context", "create", "test-docker", "--from", "default").ExecOrDie()
	output := s.NewCommand("docker", "context", "ls").ExecOrDie()
	golden.Assert(s.T(), output, GoldenFile("ls-out-test-docker"))
}

func (s *E2eSuite) TestContextListQuiet() {
	s.NewDockerCommand("context", "create", "test-docker", "--from", "default").ExecOrDie()
	output := s.NewCommand("docker", "context", "ls", "-q").ExecOrDie()
	Expect(output).To(Equal(`default
test-docker
`))
}

func (s *E2eSuite) TestInspectDefaultContext() {
	output := s.NewDockerCommand("context", "inspect", "default").ExecOrDie()
	Expect(output).To(ContainSubstring(`"Name": "default"`))
}

func (s *E2eSuite) TestInspectContextNoArgs() {
	output := s.NewDockerCommand("context", "inspect").ExecOrDie()
	Expect(output).To(ContainSubstring(`"Name": "default"`))
}

func (s *E2eSuite) TestInspectContextRegardlessCurrentContext() {
	s.NewDockerCommand("context", "create", "local", "localCtx").ExecOrDie()
	s.NewDockerCommand("context", "use", "localCtx").ExecOrDie()
	output := s.NewDockerCommand("context", "inspect").ExecOrDie()
	Expect(output).To(ContainSubstring(`"Name": "localCtx"`))
}

func (s *E2eSuite) TestContextLsFormat() {
	output, err := s.NewDockerCommand("context", "ls", "--format", "{{ json . }}").Exec()
	Expect(err).To(BeNil())
	Expect(output).To(ContainSubstring(`"Name":"default"`))
}

func (s *E2eSuite) TestContextCreateParseErrorDoesNotDelegateToLegacy() {
	s.Step("should dispay new cli error when parsing context create flags", func() {
		_, err := s.NewDockerCommand("context", "create", "aci", "--subscription-id", "titi").Exec()
		Expect(err.Error()).NotTo(ContainSubstring("unknown flag"))
		Expect(err.Error()).To(ContainSubstring("accepts 1 arg(s), received 0"))
	})
}

func (s *E2eSuite) TestCannotRemoveCurrentContext() {
	s.NewDockerCommand("context", "create", "test-context-rm", "--from", "default").ExecOrDie()
	s.NewDockerCommand("context", "use", "test-context-rm").ExecOrDie()
	_, err := s.NewDockerCommand("context", "rm", "test-context-rm").Exec()
	Expect(err.Error()).To(ContainSubstring("cannot delete current context"))
}

func (s *E2eSuite) TestCanForceRemoveCurrentContext() {
	s.NewDockerCommand("context", "create", "test-context-rmf", "--from", "default").ExecOrDie()
	s.NewDockerCommand("context", "use", "test-context-rmf").ExecOrDie()
	s.NewDockerCommand("context", "rm", "-f", "test-context-rmf").ExecOrDie()
	out := s.NewDockerCommand("context", "ls").ExecOrDie()
	Expect(out).To(ContainSubstring("default *"))
}

func (s *E2eSuite) TestContextCreateAciChecksContextNameBeforeInteractivePart() {
	s.NewDockerCommand("context", "create", "mycontext", "--from", "default").ExecOrDie()
	_, err := s.NewDockerCommand("context", "create", "aci", "mycontext").Exec()
	Expect(err.Error()).To(ContainSubstring("context mycontext: already exists"))
}

func (s *E2eSuite) TestClassicLoginWithparameters() {
	output, err := s.NewDockerCommand("login", "-u", "nouser", "-p", "wrongpasword").Exec()
	Expect(output).To(ContainSubstring("Get https://registry-1.docker.io/v2/: unauthorized: incorrect username or password"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestClassicLoginRegardlessCurrentContext() {
	s.NewDockerCommand("context", "create", "local", "localCtx").ExecOrDie()
	s.NewDockerCommand("context", "use", "localCtx").ExecOrDie()
	output, err := s.NewDockerCommand("login", "-u", "nouser", "-p", "wrongpasword").Exec()
	Expect(output).To(ContainSubstring("Get https://registry-1.docker.io/v2/: unauthorized: incorrect username or password"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestClassicLogin() {
	output, err := s.NewDockerCommand("login", "someregistry.docker.io").Exec()
	Expect(output).To(ContainSubstring("Cannot perform an interactive login from a non TTY device"))
	Expect(err).NotTo(BeNil())
	output, err = s.NewDockerCommand("logout", "someregistry.docker.io").Exec()
	Expect(output).To(ContainSubstring("Removing login credentials for someregistry.docker.io"))
	Expect(err).To(BeNil())
}

func (s *E2eSuite) TestCloudLogin() {
	output, err := s.NewDockerCommand("login", "mycloudbackend").Exec()
	Expect(output).To(ContainSubstring("unknown backend type for cloud login: mycloudbackend"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestSetupError() {
	s.Step("should display an error if cannot shell out to com.docker.cli", func() {
		err := os.Setenv("PATH", s.BinDir)
		Expect(err).To(BeNil())
		err = os.Remove(filepath.Join(s.BinDir, DockerClassicExecutable()))
		Expect(err).To(BeNil())
		output, err := s.NewDockerCommand("ps").Exec()
		Expect(output).To(ContainSubstring("com.docker.cli"))
		Expect(output).To(ContainSubstring("not found"))
		Expect(err).NotTo(BeNil())
	})
}

func (s *E2eSuite) TestLegacy() {
	s.Step("should list all legacy commands", func() {
		output := s.NewDockerCommand("--help").ExecOrDie()
		Expect(output).To(ContainSubstring("swarm"))
	})

	s.Step("should execute legacy commands", func() {
		output, _ := s.NewDockerCommand("swarm", "join").Exec()
		Expect(output).To(ContainSubstring("\"docker swarm join\" requires exactly 1 argument."))
	})

	s.Step("should run local container in less than 10 secs", func() {
		s.NewDockerCommand("pull", "hello-world").ExecOrDie()
		output := s.NewDockerCommand("run", "--rm", "hello-world").WithTimeout(time.NewTimer(20 * time.Second).C).ExecOrDie()
		Expect(output).To(ContainSubstring("Hello from Docker!"))
	})

	s.Step("should execute legacy commands in other moby contexts", func() {
		s.NewDockerCommand("context", "create", "mobyCtx", "--from=default").ExecOrDie()
		s.NewDockerCommand("context", "use", "mobyCtx").ExecOrDie()
		output, _ := s.NewDockerCommand("swarm", "join").Exec()
		Expect(output).To(ContainSubstring("\"docker swarm join\" requires exactly 1 argument."))
	})
}

func (s *E2eSuite) TestLeaveLegacyErrorMessagesUnchanged() {
	output, err := s.NewDockerCommand("foo").Exec()
	golden.Assert(s.T(), output, "unknown-foo-command.golden")
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestPassThroughRootLegacyFlags() {
	output, err := s.NewDockerCommand("-H", "tcp://localhost:123", "version").Exec()
	Expect(err).NotTo(BeNil())
	Expect(output).NotTo(ContainSubstring("unknown shorthand flag"))
	Expect(output).To(ContainSubstring("localhost:123"))

	output, _ = s.NewDockerCommand("-H", "tcp://localhost:123", "login", "-u", "nouser", "-p", "wrongpasword").Exec()
	Expect(output).NotTo(ContainSubstring("unknown shorthand flag"))
	Expect(output).To(ContainSubstring("WARNING! Using --password via the CLI is insecure"))

	output, _ = s.NewDockerCommand("--log-level", "debug", "login", "-u", "nouser", "-p", "wrongpasword").Exec()
	Expect(output).NotTo(ContainSubstring("unknown shorthand flag"))
	Expect(output).To(ContainSubstring("WARNING! Using --password via the CLI is insecure"))

	output, _ = s.NewDockerCommand("login", "--help").Exec()
	Expect(output).NotTo(ContainSubstring("--log-level"))
}

func (s *E2eSuite) TestDisplayFriendlyErrorMessageForLegacyCommands() {
	s.NewDockerCommand("context", "create", "example", "test-example").ExecOrDie()
	output, err := s.NewDockerCommand("--context", "test-example", "images").Exec()
	Expect(output).To(Equal("Command \"images\" not available in current context (test-example), you can use the \"default\" context to run this command\n"))
	Expect(err).NotTo(BeNil())
}

func (s *E2eSuite) TestExecMobyIfUsingHostFlag() {
	s.NewDockerCommand("context", "create", "example", "test-example").ExecOrDie()
	s.NewDockerCommand("context", "use", "test-example").ExecOrDie()
	output, err := s.NewDockerCommand("-H", defaultEndpoint(), "ps").Exec()
	Expect(err).To(BeNil())
	Expect(output).To(ContainSubstring("CONTAINER ID"))
}

func defaultEndpoint() string {
	if runtime.GOOS == "windows" {
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}

func (s *E2eSuite) TestExecMobyIfUsingversionFlag() {
	s.NewDockerCommand("context", "create", "example", "test-example").ExecOrDie()
	s.NewDockerCommand("context", "use", "test-example").ExecOrDie()
	output, err := s.NewDockerCommand("-v").Exec()
	Expect(err).To(BeNil())
	Expect(output).To(ContainSubstring("Docker version"))
}

func (s *E2eSuite) TestDisplaysAdditionalLineInDockerVersion() {
	output := s.NewDockerCommand("version").ExecOrDie()
	Expect(output).To(ContainSubstring("Azure integration"))
}

func (s *E2eSuite) TestAllowsFormatFlagInVersion() {
	s.NewDockerCommand("version", "-f", "{{ json . }}").ExecOrDie()
	s.NewDockerCommand("version", "--format", "{{ json . }}").ExecOrDie()
}

func (s *E2eSuite) TestMockBackend() {
	s.Step("creates a new test context to hardcoded example backend", func() {
		s.NewDockerCommand("context", "create", "example", "test-example").ExecOrDie()
		// Expect(output).To(ContainSubstring("test-example context acitest created"))
	})

	s.Step("uses the test context", func() {
		currentContext := s.NewDockerCommand("context", "use", "test-example").ExecOrDie()
		Expect(currentContext).To(ContainSubstring("test-example"))
		output := s.NewDockerCommand("context", "ls").ExecOrDie()
		golden.Assert(s.T(), output, GoldenFile("ls-out-test-example"))
		output = s.NewDockerCommand("context", "show").ExecOrDie()
		Expect(output).To(ContainSubstring("test-example"))
	})

	s.Step("can run ps command", func() {
		output := s.NewDockerCommand("ps").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(3))
		Expect(lines[2]).To(ContainSubstring("1234                alpine"))
	})

	s.Step("can run quiet ps command", func() {
		output := s.NewDockerCommand("ps", "-q").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(2))
		Expect(lines[0]).To(Equal("id"))
		Expect(lines[1]).To(Equal("1234"))
	})

	s.Step("can run ps command with all ", func() {
		output := s.NewDockerCommand("ps", "-q", "--all").ExecOrDie()
		lines := Lines(output)
		Expect(len(lines)).To(Equal(3))
		Expect(lines[0]).To(Equal("id"))
		Expect(lines[1]).To(Equal("1234"))
		Expect(lines[2]).To(Equal("stopped"))
	})

	s.Step("can run inspect command on container", func() {
		golden.Assert(s.T(), s.NewDockerCommand("inspect", "id").ExecOrDie(), "inspect-id.golden")
	})

	s.Step("can run 'run' command", func() {
		output := s.NewDockerCommand("run", "-d", "nginx", "-p", "80:80").ExecOrDie()
		Expect(output).To(ContainSubstring("Running container \"nginx\" with name"))
	})
}

func TestE2e(t *testing.T) {
	suite.Run(t, new(E2eSuite))
}
