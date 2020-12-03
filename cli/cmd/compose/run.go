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

	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	apicontext "github.com/docker/compose-cli/context"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/progress"
)

type runOptions struct {
	Name        string
	Command     []string
	WorkingDir  string
	Environment []string
	Detach      bool
	Publish     []string
	Labels      []string
	Volumes     []string
	NoDeps      bool
	Remove      bool
}

func runCommand() *cobra.Command {
	opts := runOptions{}
	runCmd := &cobra.Command{
		Use:   "run [options] [-v VOLUME...] [-p PORT...] [-e KEY=VAL...] [-l KEY=VALUE...] SERVICE [COMMAND] [ARGS...]",
		Short: "Run a one-off command on a service.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := store.ContextStore(cmd.Context())
			currentCtx, err := s.Get(apicontext.CurrentContext(cmd.Context()))
			if err != nil {
				return err
			}
			switch currentCtx.Type() {
			case store.DefaultContextType:
			default:
				return fmt.Errorf(`Command "run" is not yet implemented for %q context type`, currentCtx.Type())
			}

			if len(args) > 1 {
				opts.Command = args[1:]
			}
			opts.Name = args[0]
			return runRun(cmd.Context(), opts)
		},
	}
	runCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")

	runCmd.Flags().StringArrayVarP(&opts.Publish, "publish", "p", []string{}, "Publish a container's port(s). [HOST_PORT:]CONTAINER_PORT")
	runCmd.Flags().StringVar(&opts.Name, "name", "", "Assign a name to the container")
	runCmd.Flags().BoolVar(&opts.NoDeps, "no-deps", false, "Don't start linked services.")
	runCmd.Flags().StringArrayVarP(&opts.Labels, "label", "l", []string{}, "Set meta data on a container")
	runCmd.Flags().StringArrayVarP(&opts.Volumes, "volume", "v", []string{}, "Volume. Ex: storageaccount/my_share[:/absolute/path/to/target][:ro]")
	runCmd.Flags().BoolVarP(&opts.Detach, "detach", "d", false, "Run container in background and print container ID")
	runCmd.Flags().StringArrayVarP(&opts.Environment, "env", "e", []string{}, "Set environment variables")
	runCmd.Flags().BoolVar(&opts.Remove, "rm", false, "Automatically remove the container when it exits")

	//addComposeCommonFlags(runCmd.Flags(), &opts.ComposeOpts)

	runCmd.Flags().SetInterspersed(false)
	return runCmd
}

func runRun(ctx context.Context, opts runOptions) error {
	// target service
	services := []string{opts.Name}

	projectOpts := composeOptions{}
	options, err := projectOpts.toProjectOptions()
	if err != nil {
		return err
	}
	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return err
	}

	err = filter(project, services)
	if err != nil {
		return err
	}

	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}
	containerID, err := progress.Run(ctx, func(ctx context.Context) (string, error) {
		return c.ComposeService().CreateOneOffContainer(ctx, project, compose.RunOptions{
			Name:    opts.Name,
			Command: opts.Command,
		})
	})
	if err != nil {
		return err
	}
	// start container and attach to container streams
	err = c.ComposeService().Run(ctx, containerID, opts.Detach)
	if err != nil {
		return err
	}
	if opts.Detach {
		fmt.Printf("%s", containerID)
		return nil
	}
	if opts.Remove {
		return c.ContainerService().Delete(ctx, containerID, containers.DeleteRequest{
			Force: true,
		})
	}
	return nil
}
