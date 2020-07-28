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

package mobycli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

// ComDockerCli name of the classic cli binary
const ComDockerCli = "com.docker.cli"

// ExecIfDefaultCtxType delegates to com.docker.cli if on moby or AWS context (until there is an AWS backend)
func ExecIfDefaultCtxType(ctx context.Context) {
	currentContext := apicontext.CurrentContext(ctx)

	s := store.ContextStore(ctx)

	currentCtx, err := s.Get(currentContext)
	// Only run original docker command if the current context is not ours.
	if err != nil || mustDelegateToMoby(currentCtx.Type()) {
		Exec(ctx)
	}
}

func mustDelegateToMoby(ctxType string) bool {
	return ctxType == store.DefaultContextType || ctxType == store.AwsContextType
}

// Exec delegates to com.docker.cli if on moby context
func Exec(ctx context.Context) {
	if os.Args[1] == "compose" {
		// command is not implemented for moby or aws context
		fmt.Fprintln(os.Stderr, errors.New("'compose' command is not implemented for the context in use"))
		os.Exit(1)
	}

	cmd := exec.CommandContext(ctx, ComDockerCli, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			os.Exit(exiterr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// ExecCmd delegates the cli command to com.docker.cli. The error is never
// returned (process will exit with docker classic exit code), the return type
// is to make it easier to use with cobra commands
func ExecCmd(command *cobra.Command) error {
	Exec(command.Context())
	return nil
}

// IsDefaultContextCommand checks if the command exists in the classic cli (issues a shellout --help)
func IsDefaultContextCommand(dockerCommand string) bool {
	cmd := exec.Command(ComDockerCli, dockerCommand, "--help")
	b, e := cmd.CombinedOutput()
	if e != nil {
		fmt.Println(e)
	}
	output := string(b)
	contains := strings.Contains(output, "Usage:\tdocker "+dockerCommand)
	return contains
}

// ExecSilent executes a command and do redirect output to stdOut, return output
func ExecSilent(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ComDockerCli, os.Args[1:]...)
	return cmd.CombinedOutput()
}
