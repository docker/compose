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

	"github.com/docker/compose-cli/cli/cmd/compose"
	"github.com/docker/compose-cli/cli/cmd/logout"
	volume "github.com/docker/compose-cli/cli/cmd/volume"
	"github.com/docker/compose-cli/errdefs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// Backend registrations
	_ "github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/cli/cmd"
	contextcmd "github.com/docker/compose-cli/cli/cmd/context"
	"github.com/docker/compose-cli/cli/cmd/login"
	"github.com/docker/compose-cli/cli/cmd/run"
	"github.com/docker/compose-cli/cli/mobycli"
	cliopts "github.com/docker/compose-cli/cli/options"
	"github.com/docker/compose-cli/config"
	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/store"
	_ "github.com/docker/compose-cli/ecs"
	_ "github.com/docker/compose-cli/ecs/local"
	_ "github.com/docker/compose-cli/example"
	_ "github.com/docker/compose-cli/local"
	"github.com/docker/compose-cli/metrics"
)

var (
	version = "dev"
)

var (
	contextAgnosticCommands = map[string]struct{}{
		"compose": {},
		"context": {},
		"login":   {},
		"logout":  {},
		"serve":   {},
		"version": {},
	}
	unknownCommandRegexp = regexp.MustCompile(`unknown command "([^"]*)"`)
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

func isContextAgnosticCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if _, ok := contextAgnosticCommands[cmd.Name()]; ok {
		return true
	}
	return isContextAgnosticCommand(cmd.Parent())
}

func main() {
	var opts cliopts.GlobalOpts
	root := &cobra.Command{
		Use:           "docker",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !isContextAgnosticCommand(cmd) {
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
		cmd.StartCommand(),
		cmd.InspectCommand(),
		login.Command(),
		logout.Command(),
		cmd.VersionCommand(version),
		cmd.StopCommand(),
		cmd.KillCommand(),
		cmd.SecretCommand(),
		compose.Command(),

		// Place holders
		cmd.EcsCommand(),
	)

	helpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !isContextAgnosticCommand(cmd) {
			mobycli.ExecIfDefaultCtxType(cmd.Context())
		}
		helpFunc(cmd, args)
	})

	root.PersistentFlags().BoolVarP(&opts.Debug, "debug", "D", false, "enable debug output in the logs")
	root.PersistentFlags().StringVarP(&opts.Host, "host", "H", "", "Daemon socket(s) to connect to")
	opts.AddConfigFlags(root.PersistentFlags())
	opts.AddContextFlags(root.PersistentFlags())
	root.Flags().BoolVarP(&opts.Version, "version", "v", false, "Print version information and quit")

	// populate the opts with the global flags
	_ = root.PersistentFlags().Parse(os.Args[1:])
	if opts.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx, cancel := newSigContext()
	defer cancel()

	// --host and --version should immediately be forwarded to the original cli
	if opts.Host != "" || opts.Version {
		mobycli.Exec()
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

	ctype := store.DefaultContextType
	cc, _ := s.Get(currentContext)
	if cc != nil {
		ctype = cc.Type()
	}

	if ctype == store.AciContextType {
		// we can also pass ctype as a parameter to the volume command and customize subcommands, flags, etc. when we have other backend implementations
		root.AddCommand(volume.ACICommand())
	}

	metrics.Track(ctype, os.Args[1:], root.PersistentFlags())

	ctx = apicontext.WithCurrentContext(ctx, currentContext)
	ctx = store.WithContextStore(ctx, s)

	if err = root.ExecuteContext(ctx); err != nil {
		// if user canceled request, simply exit without any error message
		if errors.Is(ctx.Err(), context.Canceled) {
			os.Exit(130)
		}
		if ctype == store.AwsContextType {
			exit(root, currentContext, errors.Errorf(`%q context type has been renamed. Recreate the context by running: 
$ docker context create %s <name>`, cc.Type(), store.EcsContextType))
		}

		// Context should always be handled by new CLI
		requiredCmd, _, _ := root.Find(os.Args[1:])
		if requiredCmd != nil && isContextAgnosticCommand(requiredCmd) {
			exit(root, currentContext, err)
		}
		mobycli.ExecIfDefaultCtxType(ctx)

		checkIfUnknownCommandExistInDefaultContext(err, currentContext)

		exit(root, currentContext, err)
	}
}

func exit(cmd *cobra.Command, ctx string, err error) {
	if errors.Is(err, errdefs.ErrLoginRequired) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(errdefs.ExitCodeLoginRequired)
	}
	if errors.Is(err, errdefs.ErrNotImplemented) {
		cmd, _, _ := cmd.Traverse(os.Args[1:])
		name := cmd.Name()
		parent := cmd.Parent()
		if parent != nil && parent.Parent() != nil {
			name = parent.Name() + " " + name
		}
		fmt.Fprintf(os.Stderr, "Command %q not available in current context (%s)\n", name, ctx)
		os.Exit(1)
	}

	fatal(err)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func checkIfUnknownCommandExistInDefaultContext(err error, currentContext string) {
	submatch := unknownCommandRegexp.FindSubmatch([]byte(err.Error()))
	if len(submatch) == 2 {
		dockerCommand := string(submatch[1])

		if mobycli.IsDefaultContextCommand(dockerCommand) {
			fmt.Fprintf(os.Stderr, "Command %q not available in current context (%s), you can use the \"default\" context to run this command\n", dockerCommand, currentContext)
			os.Exit(1)
		}
	}
}

func newSigContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s := make(chan os.Signal, 1)
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
