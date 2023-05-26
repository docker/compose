/*
   Copyright 2022 Docker Compose CLI authors

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

package cucumber

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/loader"
	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/mattn/go-shellwords"
	"gotest.tools/v3/icmd"

	"github.com/docker/compose/v2/pkg/e2e"
)

func TestCucumber(t *testing.T) {
	testingOptions := godog.Options{
		TestingT: t,
		Paths:    []string{"./cucumber-features"},
		Output:   colors.Colored(os.Stdout),
		Format:   "pretty",
	}

	status := godog.TestSuite{
		Name:                "godogs",
		Options:             &testingOptions,
		ScenarioInitializer: setup,
	}.Run()

	if status == 2 {
		t.SkipNow()
	}

	if status != 0 {
		t.Fatalf("zero status code expected, %d received", status)
	}
}

func setup(s *godog.ScenarioContext) {
	t := s.TestingT()
	projectName := loader.NormalizeProjectName(strings.Split(t.Name(), "/")[1])
	cli := e2e.NewCLI(t, e2e.WithEnv(
		fmt.Sprintf("COMPOSE_PROJECT_NAME=%s", projectName),
	))
	th := testHelper{
		T:           t,
		CLI:         cli,
		ProjectName: projectName,
	}

	s.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		cli.RunDockerComposeCmd(t, "down", "--remove-orphans", "-v", "-t", "0")
		return ctx, nil
	})

	s.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		cli.RunDockerComposeCmd(t, "down", "--remove-orphans", "-v", "-t", "0")
		return ctx, nil
	})

	s.Step(`^a compose file$`, th.setComposeFile)
	s.Step(`^a dockerfile$`, th.setDockerfile)
	s.Step(`^I run "compose (.*)"$`, th.runComposeCommand)
	s.Step(`^I run "docker (.*)"$`, th.runDockerCommand)
	s.Step(`service "(.*)" is "(.*)"$`, th.serviceIsStatus)
	s.Step(`output contains "(.*)"$`, th.outputContains(true))
	s.Step(`output does not contain "(.*)"$`, th.outputContains(false))
	s.Step(`exit code is (\d+)$`, th.exitCodeIs)
	s.Step(`a process listening on port (\d+)$`, th.listenerOnPort)
}

type testHelper struct {
	T               *testing.T
	ProjectName     string
	ComposeFile     string
	TestDir         string
	CommandOutput   string
	CommandExitCode int
	CLI             *e2e.CLI
}

func (th *testHelper) serviceIsStatus(service, status string) error {
	serviceContainerName := fmt.Sprintf("%s-%s-1", strings.ToLower(th.ProjectName), service)
	statusRegex := fmt.Sprintf("%s.*%s", serviceContainerName, status)
	res := th.CLI.RunDockerComposeCmd(th.T, "ps", "-a")
	r, _ := regexp.Compile(statusRegex)
	if !r.MatchString(res.Combined()) {
		return fmt.Errorf("Missing/incorrect ps output:\n%s\nregex:\n%s", res.Combined(), statusRegex)
	}
	return nil
}

func (th *testHelper) outputContains(expected bool) func(string) error {
	return func(substring string) error {
		contains := strings.Contains(th.CommandOutput, substring)
		if contains && !expected {
			return fmt.Errorf("Unexpected substring in output: %s\noutput: %s", substring, th.CommandOutput)
		} else if !contains && expected {
			return fmt.Errorf("Missing substring in output: %s\noutput: %s", substring, th.CommandOutput)
		}
		return nil
	}
}

func (th *testHelper) exitCodeIs(exitCode int) error {
	if exitCode != th.CommandExitCode {
		return fmt.Errorf("Wrong exit code: %d expected: %d || command output: %s", th.CommandExitCode, exitCode, th.CommandOutput)
	}
	return nil
}

func (th *testHelper) runComposeCommand(command string) error {
	commandArgs, err := shellwords.Parse(command)
	if err != nil {
		return err
	}
	commandArgs = append([]string{"-f", "-"}, commandArgs...)

	cmd := th.CLI.NewDockerComposeCmd(th.T, commandArgs...)
	cmd.Stdin = strings.NewReader(th.ComposeFile)
	cmd.Dir = th.TestDir
	res := icmd.RunCmd(cmd)
	th.CommandOutput = res.Combined()
	th.CommandExitCode = res.ExitCode
	return nil
}

func (th *testHelper) runDockerCommand(command string) error {
	commandArgs, err := shellwords.Parse(command)
	if err != nil {
		return err
	}

	cmd := th.CLI.NewDockerCmd(th.T, commandArgs...)
	cmd.Dir = th.TestDir
	res := icmd.RunCmd(cmd)
	th.CommandOutput = res.Combined()
	th.CommandExitCode = res.ExitCode
	return nil
}

func (th *testHelper) setComposeFile(composeString string) error {
	th.ComposeFile = composeString
	return nil
}

func (th *testHelper) setDockerfile(dockerfileString string) error {
	tempDir := th.T.TempDir()
	th.TestDir = tempDir

	err := os.WriteFile(filepath.Join(tempDir, "Dockerfile"), []byte(dockerfileString), 0o644)
	if err != nil {
		return err
	}
	return nil
}

func (th *testHelper) listenerOnPort(port int) error {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}

	th.T.Cleanup(func() {
		_ = l.Close()
	})

	return nil
}
