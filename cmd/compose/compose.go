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

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	composegoutils "github.com/compose-spec/compose-go/v2/utils"
	"github.com/docker/buildx/util/logutil"
	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/metadata"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/pkg/kvfile"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	ui "github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/remote"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/morikuni/aec"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	// ComposeParallelLimit set the limit running concurrent operation on docker engine
	ComposeParallelLimit = "COMPOSE_PARALLEL_LIMIT"
	// ComposeProjectName define the project name to be used, instead of guessing from parent directory
	ComposeProjectName = "COMPOSE_PROJECT_NAME"
	// ComposeCompatibility try to mimic compose v1 as much as possible
	ComposeCompatibility = "COMPOSE_COMPATIBILITY"
	// ComposeRemoveOrphans remove "orphaned" containers, i.e. containers tagged for current project but not declared as service
	ComposeRemoveOrphans = "COMPOSE_REMOVE_ORPHANS"
	// ComposeIgnoreOrphans ignore "orphaned" containers
	ComposeIgnoreOrphans = "COMPOSE_IGNORE_ORPHANS"
	// ComposeEnvFiles defines the env files to use if --env-file isn't used
	ComposeEnvFiles = "COMPOSE_ENV_FILES"
	// ComposeMenu defines if the navigation menu should be rendered. Can be also set via --menu
	ComposeMenu = "COMPOSE_MENU"
	// ComposeProgress defines type of progress output, if --progress isn't used
	ComposeProgress = "COMPOSE_PROGRESS"
)

// rawEnv load a dot env file using docker/cli key=value parser, without attempt to interpolate or evaluate values
func rawEnv(r io.Reader, filename string, vars map[string]string, lookup func(key string) (string, bool)) error {
	lines, err := kvfile.ParseFromReader(r, lookup)
	if err != nil {
		return fmt.Errorf("failed to parse env_file %s: %w", filename, err)
	}
	for _, line := range lines {
		key, value, _ := strings.Cut(line, "=")
		vars[key] = value
	}
	return nil
}

func init() {
	// compose evaluates env file values for interpolation
	// `raw` format allows to load env_file with the same parser used by docker run --env-file
	dotenv.RegisterFormat("raw", rawEnv)
}

// Command defines a compose CLI command as a func with args
type Command func(context.Context, []string) error

// CobraCommand defines a cobra command function
type CobraCommand func(context.Context, *cobra.Command, []string) error

// AdaptCmd adapt a CobraCommand func to cobra library
func AdaptCmd(fn CobraCommand) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())

		s := make(chan os.Signal, 1)
		signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-s
			cancel()
			signal.Stop(s)
			close(s)
		}()

		err := fn(ctx, cmd, args)
		if api.IsErrCanceled(err) || errors.Is(ctx.Err(), context.Canceled) {
			err = dockercli.StatusError{
				StatusCode: 130,
			}
		}
		if ui.Mode == ui.ModeJSON {
			err = makeJSONError(err)
		}
		return err
	}
}

// Adapt a Command func to cobra library
func Adapt(fn Command) func(cmd *cobra.Command, args []string) error {
	return AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		return fn(ctx, args)
	})
}

type ProjectOptions struct {
	ProjectName   string
	Profiles      []string
	ConfigPaths   []string
	WorkDir       string
	ProjectDir    string
	EnvFiles      []string
	Compatibility bool
	Progress      string
	Offline       bool
	All           bool
}

// ProjectFunc does stuff within a types.Project
type ProjectFunc func(ctx context.Context, project *types.Project) error

// ProjectServicesFunc does stuff within a types.Project and a selection of services
type ProjectServicesFunc func(ctx context.Context, project *types.Project, services []string) error

// WithProject creates a cobra run command from a ProjectFunc based on configured project options and selected services
func (o *ProjectOptions) WithProject(fn ProjectFunc, dockerCli command.Cli) func(cmd *cobra.Command, args []string) error {
	return o.WithServices(dockerCli, func(ctx context.Context, project *types.Project, services []string) error {
		return fn(ctx, project)
	})
}

// WithServices creates a cobra run command from a ProjectFunc based on configured project options and selected services
func (o *ProjectOptions) WithServices(dockerCli command.Cli, fn ProjectServicesFunc) func(cmd *cobra.Command, args []string) error {
	return Adapt(func(ctx context.Context, args []string) error {
		backend, err := compose.NewComposeService(dockerCli)
		if err != nil {
			return err
		}

		project, metrics, err := o.ToProject(ctx, dockerCli, backend, args, cli.WithoutEnvironmentResolution)
		if err != nil {
			return err
		}

		ctx = context.WithValue(ctx, tracing.MetricsKey{}, metrics)

		project, err = project.WithServicesEnvironmentResolved(true)
		if err != nil {
			return err
		}

		return fn(ctx, project, args)
	})
}

type jsonErrorData struct {
	Error   bool   `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

func errorAsJSON(message string) string {
	errorMessage := &jsonErrorData{
		Error:   true,
		Message: message,
	}
	marshal, err := json.Marshal(errorMessage)
	if err == nil {
		return string(marshal)
	} else {
		return message
	}
}

func makeJSONError(err error) error {
	if err == nil {
		return nil
	}
	var statusErr dockercli.StatusError
	if errors.As(err, &statusErr) {
		return dockercli.StatusError{
			StatusCode: statusErr.StatusCode,
			Status:     errorAsJSON(statusErr.Status),
		}
	}
	return fmt.Errorf("%s", errorAsJSON(err.Error()))
}

func (o *ProjectOptions) addProjectFlags(f *pflag.FlagSet) {
	f.StringArrayVar(&o.Profiles, "profile", []string{}, "Specify a profile to enable")
	f.StringVarP(&o.ProjectName, "project-name", "p", "", "Project name")
	f.StringArrayVarP(&o.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	f.StringArrayVar(&o.EnvFiles, "env-file", defaultStringArrayVar(ComposeEnvFiles), "Specify an alternate environment file")
	f.StringVar(&o.ProjectDir, "project-directory", "", "Specify an alternate working directory\n(default: the path of the, first specified, Compose file)")
	f.StringVar(&o.WorkDir, "workdir", "", "DEPRECATED! USE --project-directory INSTEAD.\nSpecify an alternate working directory\n(default: the path of the, first specified, Compose file)")
	f.BoolVar(&o.Compatibility, "compatibility", false, "Run compose in backward compatibility mode")
	f.StringVar(&o.Progress, "progress", os.Getenv(ComposeProgress), fmt.Sprintf(`Set type of progress output (%s)`, strings.Join(printerModes, ", ")))
	f.BoolVar(&o.All, "all-resources", false, "Include all resources, even those not used by services")
	_ = f.MarkHidden("workdir")
}

// get default value for a command line flag that is set by a coma-separated value in environment variable
func defaultStringArrayVar(env string) []string {
	return strings.FieldsFunc(os.Getenv(env), func(c rune) bool {
		return c == ','
	})
}

func (o *ProjectOptions) projectOrName(ctx context.Context, dockerCli command.Cli, services ...string) (*types.Project, string, error) {
	name := o.ProjectName
	var project *types.Project
	if len(o.ConfigPaths) > 0 || o.ProjectName == "" {
		backend, err := compose.NewComposeService(dockerCli)
		if err != nil {
			return nil, "", err
		}

		p, _, err := o.ToProject(ctx, dockerCli, backend, services, cli.WithDiscardEnvFile, cli.WithoutEnvironmentResolution)
		if err != nil {
			envProjectName := os.Getenv(ComposeProjectName)
			if envProjectName != "" {
				return nil, envProjectName, nil
			}
			return nil, "", err
		}
		project = p
		name = p.Name
	}
	return project, name, nil
}

func (o *ProjectOptions) toProjectName(ctx context.Context, dockerCli command.Cli) (string, error) {
	if o.ProjectName != "" {
		return o.ProjectName, nil
	}

	envProjectName := os.Getenv(ComposeProjectName)
	if envProjectName != "" {
		return envProjectName, nil
	}

	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return "", err
	}

	project, _, err := o.ToProject(ctx, dockerCli, backend, nil)
	if err != nil {
		return "", err
	}
	return project.Name, nil
}

func (o *ProjectOptions) ToModel(ctx context.Context, dockerCli command.Cli, services []string, po ...cli.ProjectOptionsFn) (map[string]any, error) {
	remotes := o.remoteLoaders(dockerCli)
	for _, r := range remotes {
		po = append(po, cli.WithResourceLoader(r))
	}

	options, err := o.toProjectOptions(po...)
	if err != nil {
		return nil, err
	}

	if o.Compatibility || utils.StringToBool(options.Environment[ComposeCompatibility]) {
		api.Separator = "_"
	}

	return options.LoadModel(ctx)
}

// ToProject loads a Compose project using the LoadProject API.
// Accepts optional cli.ProjectOptionsFn to control loader behavior.
func (o *ProjectOptions) ToProject(ctx context.Context, dockerCli command.Cli, backend api.Compose, services []string, po ...cli.ProjectOptionsFn) (*types.Project, tracing.Metrics, error) {
	var metrics tracing.Metrics
	remotes := o.remoteLoaders(dockerCli)

	// Setup metrics listener to collect project data
	metricsListener := func(event string, metadata map[string]any) {
		switch event {
		case "extends":
			metrics.CountExtends++
		case "include":
			paths := metadata["path"].(types.StringList)
			for _, path := range paths {
				var isRemote bool
				for _, r := range remotes {
					if r.Accept(path) {
						isRemote = true
						break
					}
				}
				if isRemote {
					metrics.CountIncludesRemote++
				} else {
					metrics.CountIncludesLocal++
				}
			}
		}
	}

	loadOpts := api.ProjectLoadOptions{
		ProjectName:       o.ProjectName,
		ConfigPaths:       o.ConfigPaths,
		WorkingDir:        o.ProjectDir,
		EnvFiles:          o.EnvFiles,
		Profiles:          o.Profiles,
		Services:          services,
		Offline:           o.Offline,
		All:               o.All,
		Compatibility:     o.Compatibility,
		ProjectOptionsFns: po,
		LoadListeners:     []api.LoadListener{metricsListener},
	}

	project, err := backend.LoadProject(ctx, loadOpts)
	if err != nil {
		return nil, metrics, err
	}

	return project, metrics, nil
}

func (o *ProjectOptions) remoteLoaders(dockerCli command.Cli) []loader.ResourceLoader {
	if o.Offline {
		return nil
	}
	git := remote.NewGitRemoteLoader(dockerCli, o.Offline)
	oci := remote.NewOCIRemoteLoader(dockerCli, o.Offline)
	return []loader.ResourceLoader{git, oci}
}

func (o *ProjectOptions) toProjectOptions(po ...cli.ProjectOptionsFn) (*cli.ProjectOptions, error) {
	opts := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(o.ProjectDir),
		// First apply os.Environment, always win
		cli.WithOsEnv,
	}

	if _, present := os.LookupEnv("PWD"); !present {
		if pwd, err := os.Getwd(); err != nil {
			return nil, err
		} else {
			opts = append(opts, cli.WithEnv([]string{"PWD=" + pwd}))
		}
	}

	opts = append(opts,
		// Load PWD/.env if present and no explicit --env-file has been set
		cli.WithEnvFiles(o.EnvFiles...),
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		cli.WithEnvFiles(o.EnvFiles...),
		cli.WithDotEnv,
		// eventually COMPOSE_PROFILES should have been set
		cli.WithDefaultProfiles(o.Profiles...),
		cli.WithName(o.ProjectName),
	)

	return cli.NewProjectOptions(o.ConfigPaths, append(po, opts...)...)
}

// PluginName is the name of the plugin
const PluginName = "compose"

// RunningAsStandalone detects when running as a standalone program
func RunningAsStandalone() bool {
	return len(os.Args) < 2 || os.Args[1] != metadata.MetadataSubcommandName && os.Args[1] != PluginName
}

type BackendOptions struct {
	Options []compose.Option
}

func (o *BackendOptions) Add(option compose.Option) {
	o.Options = append(o.Options, option)
}

// RootCommand returns the compose command with its child commands
func RootCommand(dockerCli command.Cli, backendOptions *BackendOptions) *cobra.Command { //nolint:gocyclo
	// filter out useless commandConn.CloseWrite warning message that can occur
	// when using a remote context that is unreachable: "commandConn.CloseWrite: commandconn: failed to wait: signal: killed"
	// https://github.com/docker/cli/blob/e1f24d3c93df6752d3c27c8d61d18260f141310c/cli/connhelper/commandconn/commandconn.go#L203-L215
	logrus.AddHook(logutil.NewFilter([]logrus.Level{
		logrus.WarnLevel,
	},
		"commandConn.CloseWrite:",
		"commandConn.CloseRead:",
	))

	opts := ProjectOptions{}
	var (
		ansi     string
		noAnsi   bool
		verbose  bool
		version  bool
		parallel int
		dryRun   bool
	)
	c := &cobra.Command{
		Short:            "Docker Compose",
		Long:             "Define and run multi-container applications with Docker",
		Use:              PluginName,
		TraverseChildren: true,
		// By default (no Run/RunE in parent c) for typos in subcommands, cobra displays the help of parent c but exit(0) !
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if version {
				return versionCommand(dockerCli).Execute()
			}
			_ = cmd.Help()
			return dockercli.StatusError{
				StatusCode: 1,
				Status:     fmt.Sprintf("unknown docker command: %q", "compose "+args[0]),
			}
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			parent := cmd.Root()
			if parent != nil {
				parentPrerun := parent.PersistentPreRunE
				if parentPrerun != nil {
					err := parentPrerun(cmd, args)
					if err != nil {
						return err
					}
				}
			}

			if verbose {
				logrus.SetLevel(logrus.TraceLevel)
			}

			err := setEnvWithDotEnv(opts)
			if err != nil {
				return err
			}
			if noAnsi {
				if ansi != "auto" {
					return errors.New(`cannot specify DEPRECATED "--no-ansi" and "--ansi". Please use only "--ansi"`)
				}
				ansi = "never"
				fmt.Fprint(os.Stderr, "option '--no-ansi' is DEPRECATED ! Please use '--ansi' instead.\n")
			}
			if v, ok := os.LookupEnv("COMPOSE_ANSI"); ok && !cmd.Flags().Changed("ansi") {
				ansi = v
			}
			formatter.SetANSIMode(dockerCli, ansi)

			if noColor, ok := os.LookupEnv("NO_COLOR"); ok && noColor != "" {
				ui.NoColor()
				formatter.SetANSIMode(dockerCli, formatter.Never)
			}

			switch ansi {
			case "never":
				ui.Mode = ui.ModePlain
			case "always":
				ui.Mode = ui.ModeTTY
			}

			var ep ui.EventProcessor
			switch opts.Progress {
			case "", ui.ModeAuto:
				switch {
				case ansi == "never":
					ui.Mode = ui.ModePlain
					ep = ui.NewPlainWriter(dockerCli.Err())
				case dockerCli.Out().IsTerminal():
					ep = ui.NewTTYWriter(dockerCli.Err())
				default:
					ep = ui.NewPlainWriter(dockerCli.Err())
				}
			case ui.ModeTTY:
				if ansi == "never" {
					return fmt.Errorf("can't use --progress tty while ANSI support is disabled")
				}
				ui.Mode = ui.ModeTTY
				ep = ui.NewTTYWriter(dockerCli.Err())

			case ui.ModePlain:
				if ansi == "always" {
					return fmt.Errorf("can't use --progress plain while ANSI support is forced")
				}
				ui.Mode = ui.ModePlain
				ep = ui.NewPlainWriter(dockerCli.Err())
			case ui.ModeQuiet, "none":
				ui.Mode = ui.ModeQuiet
				ep = ui.NewQuietWriter()
			case ui.ModeJSON:
				ui.Mode = ui.ModeJSON
				logrus.SetFormatter(&logrus.JSONFormatter{})
				ep = ui.NewJSONWriter(dockerCli.Err())
			default:
				return fmt.Errorf("unsupported --progress value %q", opts.Progress)
			}
			backendOptions.Add(compose.WithEventProcessor(ep))

			// (4) options validation / normalization
			if opts.WorkDir != "" {
				if opts.ProjectDir != "" {
					return errors.New(`cannot specify DEPRECATED "--workdir" and "--project-directory". Please use only "--project-directory" instead`)
				}
				opts.ProjectDir = opts.WorkDir
				fmt.Fprint(os.Stderr, aec.Apply("option '--workdir' is DEPRECATED at root level! Please use '--project-directory' instead.\n", aec.RedF))
			}
			for i, file := range opts.EnvFiles {
				if !filepath.IsAbs(file) {
					file, err := filepath.Abs(file)
					if err != nil {
						return err
					}
					opts.EnvFiles[i] = file
				}
			}

			composeCmd := cmd
			for composeCmd.Name() != PluginName {
				if !composeCmd.HasParent() {
					return fmt.Errorf("error parsing command line, expected %q", PluginName)
				}
				composeCmd = composeCmd.Parent()
			}

			if v, ok := os.LookupEnv(ComposeParallelLimit); ok && !composeCmd.Flags().Changed("parallel") {
				i, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("%s must be an integer (found: %q)", ComposeParallelLimit, v)
				}
				parallel = i
			}
			if parallel > 0 {
				logrus.Debugf("Limiting max concurrency to %d jobs", parallel)
				backendOptions.Add(compose.WithMaxConcurrency(parallel))
			}

			// dry run detection
			if dryRun {
				backendOptions.Add(compose.WithDryRun)
			}
			return nil
		},
	}

	c.AddCommand(
		upCommand(&opts, dockerCli, backendOptions),
		downCommand(&opts, dockerCli, backendOptions),
		startCommand(&opts, dockerCli, backendOptions),
		restartCommand(&opts, dockerCli, backendOptions),
		stopCommand(&opts, dockerCli, backendOptions),
		psCommand(&opts, dockerCli, backendOptions),
		listCommand(dockerCli, backendOptions),
		logsCommand(&opts, dockerCli, backendOptions),
		configCommand(&opts, dockerCli),
		killCommand(&opts, dockerCli, backendOptions),
		runCommand(&opts, dockerCli, backendOptions),
		removeCommand(&opts, dockerCli, backendOptions),
		execCommand(&opts, dockerCli, backendOptions),
		attachCommand(&opts, dockerCli, backendOptions),
		exportCommand(&opts, dockerCli, backendOptions),
		commitCommand(&opts, dockerCli, backendOptions),
		pauseCommand(&opts, dockerCli, backendOptions),
		unpauseCommand(&opts, dockerCli, backendOptions),
		topCommand(&opts, dockerCli, backendOptions),
		eventsCommand(&opts, dockerCli, backendOptions),
		portCommand(&opts, dockerCli, backendOptions),
		imagesCommand(&opts, dockerCli, backendOptions),
		versionCommand(dockerCli),
		buildCommand(&opts, dockerCli, backendOptions),
		pushCommand(&opts, dockerCli, backendOptions),
		pullCommand(&opts, dockerCli, backendOptions),
		createCommand(&opts, dockerCli, backendOptions),
		copyCommand(&opts, dockerCli, backendOptions),
		waitCommand(&opts, dockerCli, backendOptions),
		scaleCommand(&opts, dockerCli, backendOptions),
		statsCommand(&opts, dockerCli),
		watchCommand(&opts, dockerCli, backendOptions),
		publishCommand(&opts, dockerCli, backendOptions),
		alphaCommand(&opts, dockerCli, backendOptions),
		bridgeCommand(&opts, dockerCli),
		volumesCommand(&opts, dockerCli, backendOptions),
	)

	c.Flags().SetInterspersed(false)
	opts.addProjectFlags(c.Flags())
	c.RegisterFlagCompletionFunc( //nolint:errcheck
		"project-name",
		completeProjectNames(dockerCli, backendOptions),
	)
	c.RegisterFlagCompletionFunc( //nolint:errcheck
		"project-directory",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{}, cobra.ShellCompDirectiveFilterDirs
		},
	)
	c.RegisterFlagCompletionFunc( //nolint:errcheck
		"file",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
		},
	)
	c.RegisterFlagCompletionFunc( //nolint:errcheck
		"profile",
		completeProfileNames(dockerCli, &opts),
	)
	c.RegisterFlagCompletionFunc( //nolint:errcheck
		"progress",
		cobra.FixedCompletions(printerModes, cobra.ShellCompDirectiveNoFileComp),
	)

	c.Flags().StringVar(&ansi, "ansi", "auto", `Control when to print ANSI control characters ("never"|"always"|"auto")`)
	c.Flags().IntVar(&parallel, "parallel", -1, `Control max parallelism, -1 for unlimited`)
	c.Flags().BoolVarP(&version, "version", "v", false, "Show the Docker Compose version information")
	c.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Execute command in dry run mode")
	c.Flags().MarkHidden("version") //nolint:errcheck
	c.Flags().BoolVar(&noAnsi, "no-ansi", false, `Do not print ANSI control characters (DEPRECATED)`)
	c.Flags().MarkHidden("no-ansi") //nolint:errcheck
	c.Flags().BoolVar(&verbose, "verbose", false, "Show more output")
	c.Flags().MarkHidden("verbose") //nolint:errcheck
	return c
}

func setEnvWithDotEnv(opts ProjectOptions) error {
	options, err := cli.NewProjectOptions(opts.ConfigPaths,
		cli.WithWorkingDirectory(opts.ProjectDir),
		cli.WithOsEnv,
		cli.WithEnvFiles(opts.EnvFiles...),
		cli.WithDotEnv,
	)
	if err != nil {
		return nil
	}
	envFromFile, err := dotenv.GetEnvFromFile(composegoutils.GetAsEqualsMap(os.Environ()), options.EnvFiles)
	if err != nil {
		return nil
	}
	for k, v := range envFromFile {
		if _, ok := os.LookupEnv(k); !ok && strings.HasPrefix(k, "COMPOSE_") {
			if err = os.Setenv(k, v); err != nil {
				return nil
			}
		}
	}
	return err
}

var printerModes = []string{
	ui.ModeAuto,
	ui.ModeTTY,
	ui.ModePlain,
	ui.ModeJSON,
	ui.ModeQuiet,
}
