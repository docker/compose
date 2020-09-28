/*
   Copyright 2020 Docker Compose CLI authors

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
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/metrics"
)

var delegatedContextTypes = []string{store.DefaultContextType}

// ComDockerCli name of the classic cli binary
const ComDockerCli = "com.docker.cli"

// ExecIfDefaultCtxType delegates to com.docker.cli if on moby context
func ExecIfDefaultCtxType(ctx context.Context, root *cobra.Command) {
	currentContext := apicontext.CurrentContext(ctx)

	s := store.ContextStore(ctx)

	currentCtx, err := s.Get(currentContext)
	// Only run original docker command if the current context is not ours.
	if err != nil || mustDelegateToMoby(currentCtx.Type()) {
		Exec(root)
	}
}

func mustDelegateToMoby(ctxType string) bool {
	for _, ctype := range delegatedContextTypes {
		if ctxType == ctype {
			return true
		}
	}
	return false
}

// Exec delegates to com.docker.cli if on moby context
func Exec(root *cobra.Command) {
	cmd := exec.Command(ComDockerCli, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	signals := make(chan os.Signal, 1)
	childExit := make(chan bool)
	signal.Notify(signals) // catch all signals
	go func() {
		for {
			select {
			case sig := <-signals:
				if cmd.Process == nil {
					continue // can happen if receiving signal before the process is actually started
				}
				// nolint errcheck
				cmd.Process.Signal(sig)
			case <-childExit:
				return
			}
		}
	}()

	err := cmd.Run()
	childExit <- true
	if err != nil {
		metrics.Track(store.DefaultContextType, os.Args[1:], root.PersistentFlags(), metrics.FailureStatus)

		if exiterr, ok := err.(*exec.ExitError); ok {
			os.Exit(exiterr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	metrics.Track(store.DefaultContextType, os.Args[1:], root.PersistentFlags(), metrics.SuccessStatus)

	os.Exit(0)
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
func ExecSilent(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		args = os.Args[1:]
	}
	cmd := exec.CommandContext(ctx, ComDockerCli, args...)
	return cmd.CombinedOutput()
}
