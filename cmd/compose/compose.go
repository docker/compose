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
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/internal/desktop"
	"github.com/docker/compose/v2/internal/experimental"
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	ui "github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/remote"
	"github.com/docker/compose/v2/pkg/utils"
	buildkit "github.com/moby/buildkit/util/progress/progressui"
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
	// ComposeRemoveOrphans remove “orphaned" containers, i.e. containers tagged for current project but not declared as service
	ComposeRemoveOrphans = "COMPOSE_REMOVE_ORPHANS"
	// ComposeIgnoreOrphans ignore "orphaned" containers
	ComposeIgnoreOrphans = "COMPOSE_IGNORE_ORPHANS"
	// ComposeEnvFiles defines the env files to use if --env-file isn't used
	ComposeEnvFiles = "COMPOSE_ENV_FILES"
	// ComposeMenu defines if the navigation menu should be rendered. Can be also set via --menu
	ComposeMenu = "COMPOSE_MENU"
)

type Backend interface {
	api.Service

	SetDesktopClient(cli *desktop.Client)

	SetExperiments(experiments *experimental.State)
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
		var composeErr compose.Error
		if api.IsErrCanceled(err) || errors.Is(ctx.Err(), context.Canceled) {
			err = dockercli.StatusError{
				StatusCode: 130,
				Status:     compose.CanceledStatus,
			}
		}
		if errors.As(err, &composeErr) {
			err = dockercli.StatusError{
				StatusCode: composeErr.GetMetricsFailureCategory().ExitCode,
				Status:     err.Error(),
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
		options := []cli.ProjectOptionsFn{
			cli.WithResolvedPaths(true),
			cli.WithDiscardEnvFile,
		}

		project, metrics, err := o.ToProject(ctx, dockerCli, args, options...)
		if err != nil {
			return err
		}

		ctx = context.WithValue(ctx, tracing.MetricsKey{}, metrics)

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
	f.StringVar(&o.Progress, "progress", string(buildkit.AutoMode), fmt.Sprintf(`Set type of progress output (%s)`, strings.Join(printerModes, ", ")))
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
		p, _, err := o.ToProject(ctx, dockerCli, services, cli.WithDiscardEnvFile)
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

	project, _, err := o.ToProject(ctx, dockerCli, nil)
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

func (o *ProjectOptions) ToProject(ctx context.Context, dockerCli command.Cli, services []string, po ...cli.ProjectOptionsFn) (*types.Project, tracing.Metrics, error) { //nolint:gocyclo
	var metrics tracing.Metrics
	remotes := o.remoteLoaders(dockerCli)
	for _, r := range remotes {
		po = append(po, cli.WithResourceLoader(r))
	}

	options, err := o.toProjectOptions(po...)
	if err != nil {
		return nil, metrics, compose.WrapComposeError(err)
	}

	options.WithListeners(func(event string, metadata map[string]any) {
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
	})

	if o.Compatibility || utils.StringToBool(options.Environment[ComposeCompatibility]) {
		api.Separator = "_"
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return nil, metrics, compose.WrapComposeError(err)
	}

	if project.Name == "" {
		return nil, metrics, errors.New("project name can't be empty. Use `--project-name` to set a valid name")
	}

	project, err = project.WithServicesEnabled(services...)
	if err != nil {
		return nil, metrics, err
	}

	for name, s := range project.Services {
		s.CustomLabels = map[string]string{
			api.ProjectLabel:     project.Name,
			api.ServiceLabel:     name,
			api.VersionLabel:     api.ComposeVersion,
			api.WorkingDirLabel:  project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel:      "False", // default, will be overridden by `run` command
		}
		if len(o.EnvFiles) != 0 {
			s.CustomLabels[api.EnvironmentFileLabel] = strings.Join(o.EnvFiles, ",")
		}
		project.Services[name] = s
	}

	project, err = project.WithSelectedServices(services)
	if err != nil {
		return nil, tracing.Metrics{}, err
	}

	if !o.All {
		project = project.WithoutUnnecessaryResources()
	}
	return project, metrics, err
}

func (o *ProjectOptions) remoteLoaders(dockerCli command.Cli) []loader.ResourceLoader {
	if o.Offline {
		return nil
	}
	git := remote.NewGitRemoteLoader(o.Offline)
	oci := remote.NewOCIRemoteLoader(dockerCli, o.Offline)
	return []loader.ResourceLoader{git, oci}
}

func (o *ProjectOptions) toProjectOptions(po ...cli.ProjectOptionsFn) (*cli.ProjectOptions, error) {
	return cli.NewProjectOptions(o.ConfigPaths,
		append(po,
			cli.WithWorkingDirectory(o.ProjectDir),
			// First apply os.Environment, always win
			cli.WithOsEnv,
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
			cli.WithName(o.ProjectName))...)
}

// PluginName is the name of the plugin
const PluginName = "compose"

// RunningAsStandalone detects when running as a standalone program
func RunningAsStandalone() bool {
	return len(os.Args) < 2 || os.Args[1] != manager.MetadataSubcommandName && os.Args[1] != PluginName
}

// RootCommand returns the compose command with its child commands
func RootCommand(dockerCli command.Cli, backend Backend) *cobra.Command { //nolint:gocyclo
	// filter out useless commandConn.CloseWrite warning message that can occur
	// when using a remote context that is unreachable: "commandConn.CloseWrite: commandconn: failed to wait: signal: killed"
	// https://github.com/docker/cli/blob/e1f24d3c93df6752d3c27c8d61d18260f141310c/cli/connhelper/commandconn/commandconn.go#L203-L215
	logrus.AddHook(logutil.NewFilter([]logrus.Level{
		logrus.WarnLevel,
	},
		"commandConn.CloseWrite:",
		"commandConn.CloseRead:",
	))

	experiments := experimental.NewState()
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
				StatusCode: compose.CommandSyntaxFailure.ExitCode,
				Status:     fmt.Sprintf("unknown docker command: %q", "compose "+args[0]),
			}
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
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

			switch opts.Progress {
			case ui.ModeAuto:
				ui.Mode = ui.ModeAuto
				if ansi == "never" {
					ui.Mode = ui.ModePlain
				}
			case ui.ModeTTY:
				if ansi == "never" {
					return fmt.Errorf("can't use --progress tty while ANSI support is disabled")
				}
				ui.Mode = ui.ModeTTY
			case ui.ModePlain:
				if ansi == "always" {
					return fmt.Errorf("can't use --progress plain while ANSI support is forced")
				}
				ui.Mode = ui.ModePlain
			case ui.ModeQuiet, "none":
				ui.Mode = ui.ModeQuiet
			case ui.ModeJSON:
				ui.Mode = ui.ModeJSON
				logrus.SetFormatter(&logrus.JSONFormatter{})
			default:
				return fmt.Errorf("unsupported --progress value %q", opts.Progress)
			}

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
			for {
				if composeCmd.Name() == PluginName {
					break
				}
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
				backend.MaxConcurrency(parallel)
			}

			// dry run detection
			ctx, err = backend.DryRunMode(ctx, dryRun)
			if err != nil {
				return err
			}
			cmd.SetContext(ctx)

			// (6) Desktop integration
			var desktopCli *desktop.Client
			if !dryRun {
				if desktopCli, err = desktop.NewFromDockerClient(ctx, dockerCli); desktopCli != nil {
					logrus.Debugf("Enabled Docker Desktop integration (experimental) @ %s", desktopCli.Endpoint())
					backend.SetDesktopClient(desktopCli)
				} else if err != nil {
					// not fatal, Compose will still work but behave as though
					// it's not running as part of Docker Desktop
					logrus.Debugf("failed to enable Docker Desktop integration: %v", err)
				} else {
					logrus.Trace("Docker Desktop integration not enabled")
				}
			}

			// (7) experimental features
			if err := experiments.Load(ctx, desktopCli); err != nil {
				logrus.Debugf("Failed to query feature flags from Desktop: %v", err)
			}
			backend.SetExperiments(experiments)

			return nil
		},
	}

	c.AddCommand(
		upCommand(&opts, dockerCli, backend, experiments),
		downCommand(&opts, dockerCli, backend),
		startCommand(&opts, dockerCli, backend),
		restartCommand(&opts, dockerCli, backend),
		stopCommand(&opts, dockerCli, backend),
		psCommand(&opts, dockerCli, backend),
		listCommand(dockerCli, backend),
		logsCommand(&opts, dockerCli, backend),
		configCommand(&opts, dockerCli),
		killCommand(&opts, dockerCli, backend),
		runCommand(&opts, dockerCli, backend),
		removeCommand(&opts, dockerCli, backend),
		execCommand(&opts, dockerCli, backend),
		attachCommand(&opts, dockerCli, backend),
		pauseCommand(&opts, dockerCli, backend),
		unpauseCommand(&opts, dockerCli, backend),
		topCommand(&opts, dockerCli, backend),
		eventsCommand(&opts, dockerCli, backend),
		portCommand(&opts, dockerCli, backend),
		imagesCommand(&opts, dockerCli, backend),
		versionCommand(dockerCli),
		buildCommand(&opts, dockerCli, backend),
		pushCommand(&opts, dockerCli, backend),
		pullCommand(&opts, dockerCli, backend),
		createCommand(&opts, dockerCli, backend),
		copyCommand(&opts, dockerCli, backend),
		waitCommand(&opts, dockerCli, backend),
		scaleCommand(&opts, dockerCli, backend),
		statsCommand(&opts, dockerCli),
		watchCommand(&opts, dockerCli, backend),
		alphaCommand(&opts, dockerCli, backend),
	)

	c.Flags().SetInterspersed(false)
	opts.addProjectFlags(c.Flags())
	c.RegisterFlagCompletionFunc( //nolint:errcheck
		"project-name",
		completeProjectNames(backend),
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
		if _, ok := os.LookupEnv(k); !ok {
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

func SetUnchangedOption(name string, experimentalFlag bool) bool {
	var value bool
	// If the var is defined we use that value first
	if envVar, ok := os.LookupEnv(name); ok {
		value = utils.StringToBool(envVar)
	} else {
		// if not, we try to get it from experimental feature flag
		value = experimentalFlag
	}
	return value
}
