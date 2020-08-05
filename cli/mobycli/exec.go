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
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

var delegatedContextTypes = []string{store.DefaultContextType}

// ComDockerCli name of the classic cli binary
const ComDockerCli = "com.docker.cli"

// ExecIfDefaultCtxType delegates to com.docker.cli if on moby or AWS context (until there is an AWS backend)
func ExecIfDefaultCtxType(ctx context.Context) {
	currentContext := apicontext.CurrentContext(ctx)

	s := store.ContextStore(ctx)

	currentCtx, err := s.Get(currentContext)
	// Only run original docker command if the current context is not ours.
	if err != nil || mustDelegateToMoby(currentCtx.Type()) {
		Exec()
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
func Exec() {
	cmd := exec.Command(ComDockerCli, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	s := make(chan os.Signal)
	signal.Notify(s) // catch all signals
	go func() {
		for sig := range s {
			err := cmd.Process.Signal(sig)
			if err != nil {
				fmt.Printf("WARNING could not forward signal %s to %s : %s\n", sig.String(), ComDockerCli, err.Error())
			}
		}
	}()

	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			os.Exit(exiterr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
func ExecSilent(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ComDockerCli, os.Args[1:]...)
	return cmd.CombinedOutput()
}
