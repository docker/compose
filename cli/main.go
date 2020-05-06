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
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	_ "github.com/docker/api/azure"
	_ "github.com/docker/api/example"

	"github.com/docker/api/cli/cmd"
	"github.com/docker/api/cli/cmd/compose"
	"github.com/docker/api/cli/cmd/run"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
	"github.com/docker/api/util"
)

type mainOpts struct {
	apicontext.Flags
	debug bool
}

func init() {
	// initial hack to get the path of the project's bin dir
	// into the env of this cli for development
	path, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), path)); err != nil {
		panic(err)
	}
}

func isOwnCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Name() == "context" || cmd.Name() == "serve" {
		return true
	}
	return isOwnCommand(cmd.Parent())
}

func main() {
	var opts mainOpts
	root := &cobra.Command{
		Use:           "docker",
		Long:          "docker for the 2020s",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !isOwnCommand(cmd) {
				execMoby(cmd.Context())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(
		cmd.ContextCommand(),
		cmd.PsCommand(),
		cmd.ServeCommand(),
		run.Command(),
		cmd.ExecCommand(),
		cmd.LogsCommand(),
		compose.Command(),
	)

	helpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !isOwnCommand(cmd) {
			execMoby(cmd.Context())
		}
		helpFunc(cmd, args)
	})

	root.PersistentFlags().BoolVarP(&opts.debug, "debug", "d", false, "enable debug output in the logs")
	opts.AddFlags(root.PersistentFlags())

	// populate the opts with the global flags
	_ = root.PersistentFlags().Parse(os.Args[1:])
	if opts.debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx, cancel := util.NewSigContext()
	defer cancel()

	config, err := apicontext.LoadConfigFile(opts.Config, "config.json")
	if err != nil {
		logrus.Fatal("unable ot find configuration")
	}
	currentContext := opts.Context
	if currentContext == "" {
		currentContext = config.CurrentContext
	}
	if currentContext == "" {
		currentContext = "default"
	}

	ctx = apicontext.WithCurrentContext(ctx, currentContext)
	if err != nil {
		logrus.Fatal(err)
	}

	s, err := store.New(store.WithRoot(opts.Config))
	if err != nil {
		logrus.Fatal(err)
	}
	ctx = store.WithContextStore(ctx, s)

	if err = root.ExecuteContext(ctx); err != nil {
		execMoby(ctx)
		fmt.Println(err)
		os.Exit(1)
	}
}

func execMoby(ctx context.Context) {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	_, err := s.Get(currentContext, nil)
	// Only run original docker command if the current context is not
	// ours.
	if err != nil {
		cmd := exec.Command("docker", os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				os.Exit(exiterr.ExitCode())
			}
			os.Exit(1)
		}
		os.Exit(0)
	}
}
