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

	"github.com/docker/buildx/build"
	"github.com/docker/compose-cli/cli/mobycli"
)

func (s *composeService) windowsBuild(opts map[string]build.Options, mode string) error {
	for serviceName, options := range opts {
		imageName := serviceName
		dockerfile := options.Inputs.DockerfilePath

		if options.Inputs.DockerfilePath == "-" { // image needs to be pulled
			imageName := options.Tags[0]
			err := shellOutMoby("pull", imageName)
			if err != nil {
				return err
			}
		} else {
			cmd := &commandBuilder{
				Path: options.Inputs.ContextPath,
			}
			cmd.addParams("--build-arg", options.BuildArgs)
			cmd.addFlag("--pull", options.Pull)
			cmd.addArg("--progress", mode)

			cacheFrom := []string{}
			for _, cacheImage := range options.CacheFrom {
				cacheFrom = append(cacheFrom, cacheImage.Attrs["ref"])
			}
			cmd.addList("--cache-from", cacheFrom)
			cmd.addArg("--file", dockerfile)
			cmd.addParams("--label", options.Labels)
			cmd.addArg("--network", options.NetworkMode)
			cmd.addArg("--target", options.Target)
			cmd.addList("--add-host", options.ExtraHosts)
			cmd.addArg("--tag", imageName)

			err := shellOutMoby(cmd.getArguments()...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func shellOutMoby(args ...string) error {
	childExit := make(chan bool)
	err := mobycli.RunDocker(childExit, args...)
	childExit <- true
	return err
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
