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
	"path/filepath"
	"regexp"

	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/metrics"
	"github.com/docker/compose-cli/cli/mobycli/resolvepath"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/pkg/compose"
	"github.com/docker/compose-cli/pkg/utils"
)

var delegatedContextTypes = []string{store.DefaultContextType}

// ComDockerCli name of the classic cli binary
const ComDockerCli = "com.docker.cli"

// ExecIfDefaultCtxType delegates to com.docker.cli if on moby context
func ExecIfDefaultCtxType(ctx context.Context, root *cobra.Command) {
	currentContext := apicontext.Current()

	s := store.Instance()

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
	childExit := make(chan bool)
	err := RunDocker(childExit, os.Args[1:]...)
	childExit <- true
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			exitCode := exiterr.ExitCode()
			metrics.Track(store.DefaultContextType, os.Args[1:], compose.ByExitCode(exitCode).MetricsStatus)
			os.Exit(exitCode)
		}
		metrics.Track(store.DefaultContextType, os.Args[1:], compose.FailureStatus)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	command := metrics.GetCommand(os.Args[1:])
	if command == "build" && !metrics.HasQuietFlag(os.Args[1:]) {
		utils.DisplayScanSuggestMsg()
	}
	metrics.Track(store.DefaultContextType, os.Args[1:], compose.SuccessStatus)

	os.Exit(0)
}

// RunDocker runs a docker command, and forward signals to the shellout command (stops listening to signals when an event is sent to childExit)
func RunDocker(childExit chan bool, args ...string) error {
	execBinary, err := resolvepath.LookPath(ComDockerCli)
	if err != nil {
		execBinary = findBinary(ComDockerCli)
		if execBinary == "" {
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, "Current PATH : "+os.Getenv("PATH"))
			os.Exit(1)
		}
	}
	cmd := exec.Command(execBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	signals := make(chan os.Signal, 1)
	signal.Notify(signals) // catch all signals
	go func() {
		for {
			select {
			case sig := <-signals:
				if cmd.Process == nil {
					continue // can happen if receiving signal before the process is actually started
				}
				// In go1.14+, the go runtime issues SIGURG as an interrupt to
				// support preemptable system calls on Linux. Since we can't
				// forward that along we'll check that here.
				if isRuntimeSig(sig) {
					continue
				}
				_ = cmd.Process.Signal(sig)
			case <-childExit:
				return
			}
		}
	}()

	return cmd.Run()
}

func findBinary(filename string) string {
	currentBinaryPath, err := os.Executable()
	if err != nil {
		return ""
	}
	currentBinaryPath, err = filepath.EvalSymlinks(currentBinaryPath)
	if err != nil {
		return ""
	}
	binaryPath := filepath.Join(filepath.Dir(currentBinaryPath), filename)
	if _, err := os.Stat(binaryPath); err != nil {
		return ""
	}
	return binaryPath
}

// IsDefaultContextCommand checks if the command exists in the classic cli (issues a shellout --help)
func IsDefaultContextCommand(dockerCommand string) bool {
	cmd := exec.Command(ComDockerCli, dockerCommand, "--help")
	b, e := cmd.CombinedOutput()
	if e != nil {
		fmt.Println(e)
	}
	return regexp.MustCompile("Usage:\\s*docker\\s*" + dockerCommand).Match(b)
}

// ExecSilent executes a command and do redirect output to stdOut, return output
func ExecSilent(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		args = os.Args[1:]
	}
	cmd := exec.CommandContext(ctx, ComDockerCli, args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}
