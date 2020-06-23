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
	_ "github.com/docker/api/local"

	"github.com/docker/api/cli/cmd"
	"github.com/docker/api/cli/cmd/compose"
	contextcmd "github.com/docker/api/cli/cmd/context"
	"github.com/docker/api/cli/cmd/login"
	"github.com/docker/api/cli/cmd/run"
	"github.com/docker/api/cli/mobycli"
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
		"version": {},
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
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !isOwnCommand(cmd) {
				mobycli.ExecIfDefaultCtxType(cmd.Context())
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
		cmd.InspectCommand(),
		compose.Command(),
		login.Command(),
		cmd.VersionCommand(),
	)

	helpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !isOwnCommand(cmd) {
			mobycli.ExecIfDefaultCtxType(cmd.Context())
		}
		helpFunc(cmd, args)
	})

	root.PersistentFlags().BoolVarP(&opts.Debug, "debug", "D", false, "enable debug output in the logs")
	root.PersistentFlags().StringVarP(&opts.Host, "host", "H", "", "Daemon socket(s) to connect to")
	opts.AddConfigFlags(root.PersistentFlags())
	opts.AddContextFlags(root.PersistentFlags())

	// populate the opts with the global flags
	_ = root.PersistentFlags().Parse(os.Args[1:])
	if opts.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx, cancel := newSigContext()
	defer cancel()

	if opts.Host != "" {
		mobycli.ExecRegardlessContext(ctx)
	}

	if opts.Config == "" {
		fatal(errors.New("config path cannot be empty"))
	}
	configDir := opts.Config
	ctx = config.WithDir(ctx, configDir)

	currentContext := determineCurrentContext(opts.Context, configDir)

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
		mobycli.ExecIfDefaultCtxType(ctx)

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

		if mobycli.IsDefaultContextCommand(dockerCommand) {
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

func determineCurrentContext(flag string, configDir string) string {
	res := flag
	if res == "" {
		config, err := config.LoadFile(configDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, errors.Wrap(err, "WARNING"))
			return "default"
		}
		res = config.CurrentContext
	}
	if res == "" {
		res = "default"
	}
	return res
}

func fatal(err error) {
	fmt.Fprint(os.Stderr, err)
	os.Exit(1)
}
