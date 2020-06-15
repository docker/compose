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
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// Backend registrations
	_ "github.com/docker/api/azure"
	_ "github.com/docker/api/example"
	_ "github.com/docker/api/moby"

	"github.com/docker/api/cli/cmd"
	"github.com/docker/api/cli/cmd/compose"
	contextcmd "github.com/docker/api/cli/cmd/context"
	"github.com/docker/api/cli/cmd/login"
	"github.com/docker/api/cli/cmd/run"
	"github.com/docker/api/cli/dockerclassic"
	cliopts "github.com/docker/api/cli/options"
	"github.com/docker/api/config"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

var (
	ownCommands = map[string]struct{}{
		"context": {},
		"login":   {},
		"serve":   {},
	}
)

func init() {
	// initial hack to get the path of the project's bin dir
	// into the env of this cli for development
	path, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fatal(errors.Wrap(err, "unable to get absolute bin path"))
	}
	if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), path)); err != nil {
		panic(err)
	}
	// Seed random
	rand.Seed(time.Now().UnixNano())
}

func isOwnCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if _, ok := ownCommands[cmd.Name()]; ok {
		return true
	}
	return isOwnCommand(cmd.Parent())
}

func main() {
	var opts cliopts.GlobalOpts
	root := &cobra.Command{
		Use:           "docker",
		Long:          "docker for the 2020s",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !isOwnCommand(cmd) {
				dockerclassic.Exec(cmd.Context())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(
		contextcmd.Command(),
		cmd.PsCommand(),
		cmd.ServeCommand(),
		run.Command(),
		cmd.ExecCommand(),
		cmd.LogsCommand(),
		cmd.RmCommand(),
		compose.Command(),
		login.Command(),
	)

	helpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !isOwnCommand(cmd) {
			dockerclassic.Exec(cmd.Context())
		}
		helpFunc(cmd, args)
	})

	root.PersistentFlags().BoolVarP(&opts.Debug, "debug", "d", false, "enable debug output in the logs")
	opts.AddConfigFlags(root.PersistentFlags())
	opts.AddContextFlags(root.PersistentFlags())

	// populate the opts with the global flags
	_ = root.PersistentFlags().Parse(os.Args[1:])
	if opts.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx, cancel := newSigContext()
	defer cancel()

	if opts.Config == "" {
		fatal(errors.New("config path cannot be empty"))
	}
	configDir := opts.Config
	ctx = config.WithDir(ctx, configDir)

	currentContext, err := determineCurrentContext(opts.Context, configDir)
	if err != nil {
		fatal(errors.Wrap(err, "unable to determine current context"))
	}

	s, err := store.New(store.WithRoot(configDir))
	if err != nil {
		fatal(errors.Wrap(err, "unable to create context store"))
	}
	ctx = apicontext.WithCurrentContext(ctx, currentContext)
	ctx = store.WithContextStore(ctx, s)

	err = root.ExecuteContext(ctx)
	if err != nil {
		// Context should always be handled by new CLI
		requiredCmd, _, _ := root.Find(os.Args[1:])
		if requiredCmd != nil && isOwnCommand(requiredCmd) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		dockerclassic.Exec(ctx)

		checkIfUnknownCommandExistInDefaultContext(err, currentContext)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkIfUnknownCommandExistInDefaultContext(err error, currentContext string) {
	re := regexp.MustCompile(`unknown command "([^"]*)"`)
	submatch := re.FindSubmatch([]byte(err.Error()))
	if len(submatch) == 2 {
		dockerCommand := string(submatch[1])

		if dockerclassic.IsDefaultContextCommand(dockerCommand) {
			fmt.Fprintf(os.Stderr, "Command \"%s\" not available in current context (%s), you can use the \"default\" context to run this command\n", dockerCommand, currentContext)
			os.Exit(1)
		}
	}
}

func newSigContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s := make(chan os.Signal)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-s
		cancel()
	}()
	return ctx, cancel
}

func determineCurrentContext(flag string, configDir string) (string, error) {
	res := flag
	if res == "" {
		config, err := config.LoadFile(configDir)
		if err != nil {
			return "", err
		}
		res = config.CurrentContext
	}
	if res == "" {
		res = "default"
	}
	return res, nil
}

func fatal(err error) {
	fmt.Fprint(os.Stderr, err)
	os.Exit(1)
}
