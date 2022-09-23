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
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/docker/compose/v2/pkg/e2e"
	"gotest.tools/v3/icmd"
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
	cli := e2e.NewCLI(t, e2e.WithEnv(
		fmt.Sprintf("COMPOSE_PROJECT_NAME=%s", strings.Split(t.Name(), "/")[1]),
	))
	th := testHelper{
		T:   t,
		CLI: cli,
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
	s.Step(`^I run "compose (.*)"$`, th.runComposeCommand)
	s.Step(`service "(.*)" is "(.*)"$`, th.serviceIsStatus)
	s.Step(`output contains "(.*)"$`, th.outputContains)
	s.Step(`exit code is (\d+)$`, th.exitCodeIs)
}

type testHelper struct {
	T               *testing.T
	ComposeFile     string
	CommandOutput   string
	CommandExitCode int
	CLI             *e2e.CLI
}

func (th *testHelper) serviceIsStatus(service, status string) error {
	res := th.CLI.RunDockerComposeCmd(th.T, "ps", "-a")
	statusRegex := fmt.Sprintf("%s\\s+%s", service, status)
	r, _ := regexp.Compile(statusRegex)
	if !r.MatchString(res.Combined()) {
		return fmt.Errorf("Missing/incorrect ps output:\n%s", res.Combined())
	}
	return nil
}

func (th *testHelper) outputContains(substring string) error {
	if !strings.Contains(th.CommandOutput, substring) {
		return fmt.Errorf("Missing output substring: %s\noutput: %s", substring, th.CommandOutput)
	}
	return nil
}

func (th *testHelper) exitCodeIs(exitCode int) error {
	if exitCode != th.CommandExitCode {
		return fmt.Errorf("Wrong exit code: %d expected: %d", th.CommandExitCode, exitCode)
	}
	return nil
}

func (th *testHelper) runComposeCommand(command string) error {
	commandArgs := []string{"-f", "-"}
	commandArgs = append(commandArgs, strings.Split(command, " ")...)
	cmd := th.CLI.NewDockerComposeCmd(th.T, commandArgs...)
	cmd.Stdin = strings.NewReader(th.ComposeFile)
	res := icmd.RunCmd(cmd)
	th.CommandOutput = res.Combined()
	th.CommandExitCode = res.ExitCode
	return nil
}

func (th *testHelper) setComposeFile(composeString string) error {
	th.ComposeFile = composeString
	return nil
}
