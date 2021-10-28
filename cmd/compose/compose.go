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
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/morikuni/aec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

// Command defines a compose CLI command as a func with args
type Command func(context.Context, []string) error

// CobraCommand defines a cobra command function
type CobraCommand func(context.Context, *cobra.Command, []string) error

// AdaptCmd adapt a CobraCommand func to cobra library
func AdaptCmd(fn CobraCommand) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		contextString := fmt.Sprintf("%s", ctx)
		if !strings.HasSuffix(contextString, ".WithCancel") { // need to handle cancel
			cancellableCtx, cancel := context.WithCancel(cmd.Context())
			ctx = cancellableCtx
			s := make(chan os.Signal, 1)
			signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
			go func() {
				<-s
				cancel()
			}()
		}
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
		return err
	}
}

// Adapt a Command func to cobra library
func Adapt(fn Command) func(cmd *cobra.Command, args []string) error {
	return AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		return fn(ctx, args)
	})
}

// Warning is a global warning to be displayed to user on command failure
var Warning string

type projectOptions struct {
	ProjectName   string
	Profiles      []string
	ConfigPaths   []string
	WorkDir       string
	ProjectDir    string
	EnvFile       string
	Compatibility bool
}

// ProjectFunc does stuff within a types.Project
type ProjectFunc func(ctx context.Context, project *types.Project) error

// ProjectServicesFunc does stuff within a types.Project and a selection of services
type ProjectServicesFunc func(ctx context.Context, project *types.Project, services []string) error

// WithProject creates a cobra run command from a ProjectFunc based on configured project options and selected services
func (o *projectOptions) WithProject(fn ProjectFunc) func(cmd *cobra.Command, args []string) error {
	return o.WithServices(func(ctx context.Context, project *types.Project, services []string) error {
		return fn(ctx, project)
	})
}

// WithServices creates a cobra run command from a ProjectFunc based on configured project options and selected services
func (o *projectOptions) WithServices(fn ProjectServicesFunc) func(cmd *cobra.Command, args []string) error {
	return Adapt(func(ctx context.Context, args []string) error {
		project, err := o.toProject(args, cli.WithResolvedPaths(true))
		if err != nil {
			return err
		}

		if o.EnvFile != "" {
			var services types.Services
			for _, s := range project.Services {
				ef := o.EnvFile
				if ef != "" {
					if !filepath.IsAbs(ef) {
						ef = filepath.Join(project.WorkingDir, o.EnvFile)
					}
					if s.Labels == nil {
						s.Labels = make(map[string]string)
					}
					s.Labels[api.EnvironmentFileLabel] = ef
					services = append(services, s)
				}
			}
			project.Services = services
		}

		return fn(ctx, project, args)
	})
}

func (o *projectOptions) addProjectFlags(f *pflag.FlagSet) {
	f.StringArrayVar(&o.Profiles, "profile", []string{}, "Specify a profile to enable")
	f.StringVarP(&o.ProjectName, "project-name", "p", "", "Project name")
	f.StringArrayVarP(&o.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	f.StringVar(&o.EnvFile, "env-file", "", "Specify an alternate environment file.")
	f.StringVar(&o.ProjectDir, "project-directory", "", "Specify an alternate working directory\n(default: the path of the Compose file)")
	f.StringVar(&o.WorkDir, "workdir", "", "DEPRECATED! USE --project-directory INSTEAD.\nSpecify an alternate working directory\n(default: the path of the Compose file)")
	f.BoolVar(&o.Compatibility, "compatibility", false, "Run compose in backward compatibility mode")
	_ = f.MarkHidden("workdir")
}

func (o *projectOptions) toProjectName() (string, error) {
	if o.ProjectName != "" {
		return o.ProjectName, nil
	}

	project, err := o.toProject(nil)
	if err != nil {
		return "", err
	}
	return project.Name, nil
}

func (o *projectOptions) toProject(services []string, po ...cli.ProjectOptionsFn) (*types.Project, error) {
	options, err := o.toProjectOptions(po...)
	if err != nil {
		return nil, compose.WrapComposeError(err)
	}

	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return nil, compose.WrapComposeError(err)
	}

	if o.Compatibility || project.Environment["COMPOSE_COMPATIBILITY"] == "true" {
		compose.Separator = "_"
	}

	if len(services) > 0 {
		s, err := project.GetServices(services...)
		if err != nil {
			return nil, err
		}
		o.Profiles = append(o.Profiles, s.GetProfiles()...)
	}

	if profiles, ok := options.Environment["COMPOSE_PROFILES"]; ok {
		o.Profiles = append(o.Profiles, strings.Split(profiles, ",")...)
	}

	project.ApplyProfiles(o.Profiles)

	project.WithoutUnnecessaryResources()

	err = project.ForServices(services)
	return project, err
}

func (o *projectOptions) toProjectOptions(po ...cli.ProjectOptionsFn) (*cli.ProjectOptions, error) {
	return cli.NewProjectOptions(o.ConfigPaths,
		append(po,
			cli.WithWorkingDirectory(o.ProjectDir),
			cli.WithEnvFile(o.EnvFile),
			cli.WithDotEnv,
			cli.WithOsEnv,
			cli.WithConfigFileEnv,
			cli.WithDefaultConfigPath,
			cli.WithName(o.ProjectName))...)
}

const pluginName = "compose"

// RunningAsStandalone detects when running as a standalone program
func RunningAsStandalone() bool {
	return len(os.Args) < 2 || os.Args[1] != manager.MetadataSubcommandName && os.Args[1] != pluginName
}

// RootCommand returns the compose command with its child commands
func RootCommand(backend api.Service) *cobra.Command {
	opts := projectOptions{}
	var (
		ansi    string
		noAnsi  bool
		verbose bool
		version bool
	)
	command := &cobra.Command{
		Short:            "Docker Compose",
		Use:              pluginName,
		TraverseChildren: true,
		// By default (no Run/RunE in parent command) for typos in subcommands, cobra displays the help of parent command but exit(0) !
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if version {
				return versionCommand().Execute()
			}
			_ = cmd.Help()
			return dockercli.StatusError{
				StatusCode: compose.CommandSyntaxFailure.ExitCode,
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
			if noAnsi {
				if ansi != "auto" {
					return errors.New(`cannot specify DEPRECATED "--no-ansi" and "--ansi". Please use only "--ansi"`)
				}
				ansi = "never"
				fmt.Fprint(os.Stderr, aec.Apply("option '--no-ansi' is DEPRECATED ! Please use '--ansi' instead.\n", aec.RedF))
			}
			if verbose {
				logrus.SetLevel(logrus.TraceLevel)
			}
			formatter.SetANSIMode(ansi)
			if opts.WorkDir != "" {
				if opts.ProjectDir != "" {
					return errors.New(`cannot specify DEPRECATED "--workdir" and "--project-directory". Please use only "--project-directory" instead`)
				}
				opts.ProjectDir = opts.WorkDir
				fmt.Fprint(os.Stderr, aec.Apply("option '--workdir' is DEPRECATED at root level! Please use '--project-directory' instead.\n", aec.RedF))
			}
			return nil
		},
	}

	command.AddCommand(
		upCommand(&opts, backend),
		downCommand(&opts, backend),
		startCommand(&opts, backend),
		restartCommand(&opts, backend),
		stopCommand(&opts, backend),
		psCommand(&opts, backend),
		listCommand(backend),
		logsCommand(&opts, backend),
		convertCommand(&opts, backend),
		killCommand(&opts, backend),
		runCommand(&opts, backend),
		removeCommand(&opts, backend),
		execCommand(&opts, backend),
		pauseCommand(&opts, backend),
		unpauseCommand(&opts, backend),
		topCommand(&opts, backend),
		eventsCommand(&opts, backend),
		portCommand(&opts, backend),
		imagesCommand(&opts, backend),
		versionCommand(),
		buildCommand(&opts, backend),
		pushCommand(&opts, backend),
		pullCommand(&opts, backend),
		createCommand(&opts, backend),
		copyCommand(&opts, backend),
	)
	command.Flags().SetInterspersed(false)
	opts.addProjectFlags(command.Flags())
	command.Flags().StringVar(&ansi, "ansi", "auto", `Control when to print ANSI control characters ("never"|"always"|"auto")`)
	command.Flags().BoolVarP(&version, "version", "v", false, "Show the Docker Compose version information")
	command.Flags().MarkHidden("version") //nolint:errcheck
	command.Flags().BoolVar(&noAnsi, "no-ansi", false, `Do not print ANSI control characters (DEPRECATED)`)
	command.Flags().MarkHidden("no-ansi") //nolint:errcheck
	command.Flags().BoolVar(&verbose, "verbose", false, "Show more output")
	command.Flags().MarkHidden("verbose") //nolint:errcheck
	return command
}
