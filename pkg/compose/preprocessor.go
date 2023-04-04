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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/config"
	"gopkg.in/yaml.v2"
)

const usingExtension = "x-using"

func (s *composeService) preProcess(ctx context.Context, project *types.Project) (*types.Project, error) {
	if using, ok := project.Extensions[usingExtension]; ok {
		model, err := yaml.Marshal(project)
		if err != nil {
			return nil, err
		}

		switch v := using.(type) {
		case string:
			model, err = s.runPreProcessor(ctx, v, model)
			if err != nil {
				return nil, err
			}
		case []string:
			for _, pp := range v {
				model, err = s.runPreProcessor(ctx, pp, model)
				if err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("unsupported syntax: %s", using)
		}

		project, err := loader.Load(types.ConfigDetails{
			WorkingDir: project.WorkingDir,
			ConfigFiles: []types.ConfigFile{
				{
					Content: model,
				},
			},
			Environment: project.Environment,
		})
		if err != nil {
			return nil, err
		}

		delete(project.Extensions, usingExtension)
		return project, nil
	}
	return project, nil
}

// runPreProcessor executes a pre-processor, passing raw compose.yaml model, and return (possibly mutated) compose.yaml model
func (s *composeService) runPreProcessor(ctx context.Context, pp string, model []byte) ([]byte, error) {
	fmt.Fprintf(s.stdinfo(), "pre-processing Compose model using %q\n", pp)
	if runtime.GOOS == "windows" {
		pp += ".exe"
	}
	pluginDir, err := config.Path("compose-plugins")
	if err != nil {
		return nil, err
	}

	var executable string
	candidates := []string{
		pluginDir,
		"/usr/local/lib/docker/compose-plugins",
		"/usr/local/libexec/docker/compose-plugins",
		"/usr/lib/docker/compose-plugins",
		"/usr/libexec/docker/compose-plugins",
	}
	for _, candidate := range candidates {
		path := filepath.Join(candidate, pp)
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		executable = path
		break
	}

	if executable == "" {
		return nil, fmt.Errorf("compose plugin %s was not found in any supported location %s", pp, candidates)
	}

	out := bytes.Buffer{}
	command := exec.CommandContext(ctx, executable, string(model))
	command.Env = append(os.Environ(), "DOCKER_CONTEXT="+s.dockerCli.CurrentContext())
	command.Stderr = s.stderr()
	command.Stdout = &out
	err = command.Wait()
	return out.Bytes(), err
}
