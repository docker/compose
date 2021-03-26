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
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/local/moby"

	"github.com/compose-spec/compose-go/types"
)

func (s *composeService) windowsBuild(project *types.Project, options compose.BuildOptions) error {
	projectDir := project.WorkingDir
	for _, service := range project.Services {
		if service.Build != nil {
			imageName := getImageName(service, project.Name)
			dockerfile := service.Build.Dockerfile
			if dockerfile != "" {
				if stat, err := os.Stat(projectDir); err == nil && stat.IsDir() {

					dockerfile = filepath.Join(projectDir, dockerfile)
				}
			}
			// build args
			cmd := &commandBuilder{
				Path: filepath.Join(projectDir, service.Build.Context),
			}
			cmd.addParams("--build-arg", options.Args)
			cmd.addFlag("--pull", options.Pull)
			cmd.addArg("--progress", options.Progress)

			cmd.addList("--cache-from", service.Build.CacheFrom)
			cmd.addArg("--file", dockerfile)
			cmd.addParams("--label", service.Build.Labels)
			cmd.addArg("--network", service.Build.Network)
			cmd.addArg("--target", service.Build.Target)
			cmd.addArg("--platform", service.Platform)
			cmd.addArg("--isolation", service.Build.Isolation)
			cmd.addList("--add-host", service.Build.ExtraHosts)

			cmd.addArg("--tag", imageName)

			args := cmd.getArguments()
			// shell out to moby cli
			err := moby.Exec(args)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type commandBuilder struct {
	Args []string
	Path string
}

func (c *commandBuilder) addArg(name, value string) {
	if value != "" {
		c.Args = append(c.Args, name, value)
	}
}

func (c *commandBuilder) addFlag(name string, flag bool) {
	if flag {
		c.Args = append(c.Args, name)
	}
}

func (c *commandBuilder) addParams(name string, params map[string]string) {
	if len(params) > 0 {
		for k, v := range params {
			c.Args = append(c.Args, name, fmt.Sprintf("%s=%s", k, v))
		}
	}
}

func (c *commandBuilder) addList(name string, values []string) {
	if len(values) > 0 {
		for _, v := range values {
			c.Args = append(c.Args, name, v)
		}
	}
}

func (c *commandBuilder) getArguments() []string {
	cmd := []string{"build"}
	cmd = append(cmd, c.Args...)
	cmd = append(cmd, c.Path)
	return cmd
}
