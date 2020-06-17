package mobycli

import (
	"context"
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

// Exec delegates to com.docker.cli
func Exec(ctx context.Context) {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	_, err := s.Get(currentContext)
	// Only run original docker command if the current context is not
	// ours.
	if err != nil {
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
}

// ExecCmd delegates the cli command to com.docker.cli. The error is never returned (process will exit with docker classic exit code), the return type is to make it easier to use with cobra commands
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
