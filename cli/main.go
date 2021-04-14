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

package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	cliconfig "github.com/docker/cli/cli/config"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/backend"
	"github.com/docker/compose-cli/api/config"
	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/cli/cmd"
	"github.com/docker/compose-cli/cli/cmd/compose"
	contextcmd "github.com/docker/compose-cli/cli/cmd/context"
	"github.com/docker/compose-cli/cli/cmd/login"
	"github.com/docker/compose-cli/cli/cmd/logout"
	"github.com/docker/compose-cli/cli/cmd/run"
	"github.com/docker/compose-cli/cli/cmd/volume"
	"github.com/docker/compose-cli/cli/metrics"
	"github.com/docker/compose-cli/cli/mobycli"
	cliopts "github.com/docker/compose-cli/cli/options"
	"github.com/docker/compose-cli/local"

	// Backend registrations
	_ "github.com/docker/compose-cli/aci"
	_ "github.com/docker/compose-cli/ecs"
	_ "github.com/docker/compose-cli/ecs/local"
	_ "github.com/docker/compose-cli/local"
)

var (
	contextAgnosticCommands = map[string]struct{}{
		"compose":          {},
		"context":          {},
		"login":            {},
		"logout":           {},
		"serve":            {},
		"version":          {},
		"backend-metadata": {},
	}
	unknownCommandRegexp = regexp.MustCompile(`unknown docker command: "([^"]*)"`)
)

func init() {
	// initial hack to get the path of the project's bin dir
	// into the env of this cli for development
	path, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fatal(errors.Wrap(err, "unable to get absolute bin path"))
	}

	if err := os.Setenv("PATH", appendPaths(os.Getenv("PATH"), path)); err != nil {
		panic(err)
	}
	// Seed random
	rand.Seed(time.Now().UnixNano())
}

func appendPaths(envPath string, path string) string {
	if envPath == "" {
		return path
	}
	return strings.Join([]string{envPath, path}, string(os.PathListSeparator))
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
		Use:              "docker",
		SilenceErrors:    true,
		SilenceUsage:     true,
		TraverseChildren: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !isContextAgnosticCommand(cmd) {
				mobycli.ExecIfDefaultCtxType(cmd.Context(), cmd.Root())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return fmt.Errorf("unknown docker command: %q", args[0])
		},
	}

	root.AddCommand(
		contextcmd.Command(),
		cmd.PsCommand(),
		cmd.ServeCommand(),
		cmd.ExecCommand(),
		cmd.LogsCommand(),
		cmd.RmCommand(),
		cmd.StartCommand(),
		cmd.InspectCommand(),
		login.Command(),
		logout.Command(),
		cmd.VersionCommand(),
		cmd.StopCommand(),
		cmd.KillCommand(),
		cmd.SecretCommand(),
		cmd.PruneCommand(),
		cmd.MetadataCommand(),

		// Place holders
		cmd.EcsCommand(),
	)

	helpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !isContextAgnosticCommand(cmd) {
			mobycli.ExecIfDefaultCtxType(cmd.Context(), cmd.Root())
		}
		helpFunc(cmd, args)
	})

	flags := root.Flags()
	opts.InstallFlags(flags)
	opts.AddConfigFlags(flags)
	flags.BoolVarP(&opts.Version, "version", "v", false, "Print version information and quit")

	flags.SetInterspersed(false)

	walk(root, func(c *cobra.Command) {
		c.Flags().BoolP("help", "h", false, "Help for "+c.Name())
	})

	// populate the opts with the global flags
	flags.Parse(os.Args[1:]) //nolint: errcheck

	level, err := logrus.ParseLevel(opts.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse logging level: %s\n", opts.LogLevel)
		os.Exit(1)
	}
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
	})
	logrus.SetLevel(level)
	if opts.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx, cancel := newSigContext()
	defer cancel()

	// --version should immediately be forwarded to the original cli
	if opts.Version {
		mobycli.Exec(root)
	}

	if opts.Config == "" {
		fatal(errors.New("config path cannot be empty"))
	}
	configDir := opts.Config
	config.WithDir(configDir)

	currentContext := determineCurrentContext(opts.Context, configDir, opts.Hosts)
	apicontext.WithCurrentContext(currentContext)

	s, err := store.New(configDir)
	if err != nil {
		mobycli.Exec(root)
	}
	store.WithContextStore(s)

	ctype := store.DefaultContextType
	cc, _ := s.Get(currentContext)
	if cc != nil {
		ctype = cc.Type()
	}

	service, err := getBackend(ctype, configDir, opts)
	if err != nil {
		fatal(err)
	}
	backend.WithBackend(service)

	root.AddCommand(
		run.Command(ctype),
		compose.Command(ctype, service.ComposeService()),
		volume.Command(ctype),
	)

	if err = root.ExecuteContext(ctx); err != nil {
		handleError(ctx, err, ctype, currentContext, cc, root)
	}
	metrics.Track(ctype, os.Args[1:], metrics.SuccessStatus)
}

func getBackend(ctype string, configDir string, opts cliopts.GlobalOpts) (backend.Service, error) {
	switch ctype {
	case store.DefaultContextType, store.LocalContextType:
		configFile, err := cliconfig.Load(configDir)
		if err != nil {
			return nil, err
		}
		options := cliflags.CommonOptions{
			Context:  opts.Context,
			Debug:    opts.Debug,
			Hosts:    opts.Hosts,
			LogLevel: opts.LogLevel,
		}

		if opts.TLSVerify {
			options.TLS = opts.TLS
			options.TLSVerify = opts.TLSVerify
			options.TLSOptions = opts.TLSOptions
		}
		apiClient, err := command.NewAPIClientFromFlags(&options, configFile)
		if err != nil {
			return nil, err
		}
		return local.NewService(apiClient), nil
	}
	service, err := backend.Get(ctype)
	if errdefs.IsNotFoundError(err) {
		return service, nil
	}
	return service, err
}

func handleError(ctx context.Context, err error, ctype string, currentContext string, cc *store.DockerContext, root *cobra.Command) {
	// if user canceled request, simply exit without any error message
	if errdefs.IsErrCanceled(err) || errors.Is(ctx.Err(), context.Canceled) {
		metrics.Track(ctype, os.Args[1:], metrics.CanceledStatus)
		os.Exit(130)
	}
	if ctype == store.AwsContextType {
		exit(currentContext, errors.Errorf(`%q context type has been renamed. Recreate the context by running:
$ docker context create %s <name>`, cc.Type(), store.EcsContextType), ctype)
	}

	// Context should always be handled by new CLI
	requiredCmd, _, _ := root.Find(os.Args[1:])
	if requiredCmd != nil && isContextAgnosticCommand(requiredCmd) {
		exit(currentContext, err, ctype)
	}
	mobycli.ExecIfDefaultCtxType(ctx, root)

	checkIfUnknownCommandExistInDefaultContext(err, currentContext, ctype)

	exit(currentContext, err, ctype)
}

func exit(ctx string, err error, ctype string) {
	if exit, ok := err.(cli.StatusError); ok {
		metrics.Track(ctype, os.Args[1:], metrics.SuccessStatus)
		os.Exit(exit.StatusCode)
	}

	var composeErr metrics.ComposeError
	metricsStatus := metrics.FailureStatus
	exitCode := 1
	if errors.As(err, &composeErr) {
		metricsStatus = composeErr.GetMetricsFailureCategory().MetricsStatus
		exitCode = composeErr.GetMetricsFailureCategory().ExitCode
	}
	if strings.HasPrefix(err.Error(), "unknown shorthand flag:") || strings.HasPrefix(err.Error(), "unknown flag:") || strings.HasPrefix(err.Error(), "unknown docker command:") {
		metricsStatus = metrics.CommandSyntaxFailure.MetricsStatus
		exitCode = metrics.CommandSyntaxFailure.ExitCode
	}
	metrics.Track(ctype, os.Args[1:], metricsStatus)

	if errors.Is(err, errdefs.ErrLoginRequired) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(errdefs.ExitCodeLoginRequired)
	}

	if compose.Warning != "" {
		fmt.Fprintln(os.Stderr, compose.Warning)
	}

	if errors.Is(err, errdefs.ErrNotImplemented) {
		name := metrics.GetCommand(os.Args[1:])
		fmt.Fprintf(os.Stderr, "Command %q not available in current context (%s)\n", name, ctx)

		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(exitCode)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func checkIfUnknownCommandExistInDefaultContext(err error, currentContext string, contextType string) {
	submatch := unknownCommandRegexp.FindSubmatch([]byte(err.Error()))
	if len(submatch) == 2 {
		dockerCommand := string(submatch[1])

		if mobycli.IsDefaultContextCommand(dockerCommand) {
			fmt.Fprintf(os.Stderr, "Command %q not available in current context (%s), you can use the \"default\" context to run this command\n", dockerCommand, currentContext)
			metrics.Track(contextType, os.Args[1:], metrics.FailureStatus)
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

func determineCurrentContext(flag string, configDir string, hosts []string) string {
	// host and context flags cannot be both set at the same time -- the local backend enforces this when resolving hostname
	// -H flag disables context --> set default as current
	if len(hosts) > 0 {
		return "default"
	}
	// DOCKER_HOST disables context --> set default as current
	if _, present := os.LookupEnv("DOCKER_HOST"); present {
		return "default"
	}
	res := flag
	if res == "" {
		// check if DOCKER_CONTEXT env variable was set
		if _, present := os.LookupEnv("DOCKER_CONTEXT"); present {
			res = os.Getenv("DOCKER_CONTEXT")
		}

		if res == "" {
			config, err := config.LoadFile(configDir)
			if err != nil {
				fmt.Fprintln(os.Stderr, errors.Wrap(err, "WARNING"))
				return "default"
			}
			res = config.CurrentContext
		}
	}
	if res == "" {
		res = "default"
	}
	return res
}

func walk(c *cobra.Command, f func(*cobra.Command)) {
	f(c)
	for _, c := range c.Commands() {
		walk(c, f)
	}
}
