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

package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/containerd/console"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/options/run"
	"github.com/docker/compose-cli/pkg/progress"
)

// Command runs a container
func Command(contextType string) *cobra.Command {
	var opts run.Opts
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a container",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				opts.Command = args[1:]
			}
			return runRun(cmd.Context(), args[0], contextType, opts)
		},
	}
	cmd.Flags().SetInterspersed(false)

	cmd.Flags().StringArrayVarP(&opts.Publish, "publish", "p", []string{}, "Publish a container's port(s). [HOST_PORT:]CONTAINER_PORT")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Assign a name to the container")
	cmd.Flags().StringArrayVarP(&opts.Labels, "label", "l", []string{}, "Set meta data on a container")
	cmd.Flags().StringArrayVarP(&opts.Volumes, "volume", "v", []string{}, "Volume. Ex: storageaccount/my_share[:/absolute/path/to/target][:ro]")
	cmd.Flags().BoolVarP(&opts.Detach, "detach", "d", false, "Run container in background and print container ID")
	cmd.Flags().Float64Var(&opts.Cpus, "cpus", 1., "Number of CPUs")
	cmd.Flags().VarP(&opts.Memory, "memory", "m", "Memory limit")
	cmd.Flags().StringArrayVarP(&opts.Environment, "env", "e", []string{}, "Set environment variables")
	cmd.Flags().StringArrayVar(&opts.EnvironmentFiles, "env-file", []string{}, "Path to environment files to be translated as environment variables")
	cmd.Flags().StringVarP(&opts.RestartPolicyCondition, "restart", "", containers.RestartPolicyRunNo, "Restart policy to apply when a container exits (no|always|on-failure)")
	cmd.Flags().BoolVar(&opts.Rm, "rm", false, "Automatically remove the container when it exits")
	cmd.Flags().StringVar(&opts.HealthCmd, "health-cmd", "", "Command to run to check health")
	cmd.Flags().DurationVar(&opts.HealthInterval, "health-interval", time.Duration(0), "Time between running the check (ms|s|m|h) (default 0s)")
	cmd.Flags().IntVar(&opts.HealthRetries, "health-retries", 0, "Consecutive failures needed to report unhealthy")
	cmd.Flags().DurationVar(&opts.HealthStartPeriod, "health-start-period", time.Duration(0), "Start period for the container to initialize before starting "+
		"health-retries countdown (ms|s|m|h) (default 0s)")
	cmd.Flags().DurationVar(&opts.HealthTimeout, "health-timeout", time.Duration(0), "Maximum time to allow one check to run (ms|s|m|h) (default 0s)")

	if contextType == store.LocalContextType {
		cmd.Flags().StringVar(&opts.Platform, "platform", os.Getenv("DOCKER_DEFAULT_PLATFORM"), "Set platform if server is multi-platform capable")
	}

	if contextType == store.AciContextType {
		cmd.Flags().StringVar(&opts.DomainName, "domainname", "", "Container NIS domain name")
	}

	switch contextType {
	case store.LocalContextType:
	default:
		_ = cmd.Flags().MarkHidden("rm")
	}

	return cmd
}

func runRun(ctx context.Context, image string, contextType string, opts run.Opts) error {
	switch contextType {
	case store.LocalContextType:
	default:
		if opts.Rm {
			return fmt.Errorf(`flag "--rm" is not yet implemented for %q context type`, contextType)
		}
	}

	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	containerConfig, err := opts.ToContainerConfig(image)
	if err != nil {
		return err
	}

	result, err := progress.RunWithStatus(ctx, func(ctx context.Context) (string, error) {
		return containerConfig.ID, c.ContainerService().Run(ctx, containerConfig)
	})
	if err != nil {
		return err
	}

	if !opts.Detach {
		var con io.Writer = os.Stdout
		req := containers.LogsRequest{
			Follow: true,
		}
		if c, err := console.ConsoleFromFile(os.Stdout); err == nil {
			size, err := c.Size()
			if err != nil {
				return err
			}
			req.Width = int(size.Width)
			con = c
		}

		req.Writer = con

		return c.ContainerService().Logs(ctx, opts.Name, req)
	}

	fmt.Println(result)
	return nil
}
