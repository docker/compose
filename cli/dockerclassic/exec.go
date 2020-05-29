package dockerclassic

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

// Exec delegates to docker-classic
func Exec(ctx context.Context) {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	_, err := s.Get(currentContext)
	// Only run original docker command if the current context is not
	// ours.
	if err != nil {
		cmd := exec.CommandContext(ctx, "docker-classic", os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				fmt.Fprintln(os.Stderr, exiterr.Error())
				os.Exit(exiterr.ExitCode())
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

// ExecCmd delegates the cli command to docker-classic. The error is never returned (process will exit with docker classic exit code), the return type is to make it easier to use with cobra commands
func ExecCmd(command *cobra.Command) error {
	Exec(command.Context())
	return nil
}
