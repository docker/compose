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
	"encoding/json"
	"os"
	"os/exec"

	"github.com/docker/buildx/build"
	_ "github.com/docker/buildx/driver/docker"           //nolint:blank-imports
	_ "github.com/docker/buildx/driver/docker-container" //nolint:blank-imports
	_ "github.com/docker/buildx/driver/kubernetes"       //nolint:blank-imports
)

type bakeConfig struct {
	Groups  map[string]bakeGroup  `json:"group"`
	Targets map[string]bakeTarget `json:"target"`
}

type bakeGroup struct {
	Targets []string `json:"targets"`
}

type bakeTarget struct {
	Context    string            `json:"context,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	CacheFrom  []string          `json:"cache-from,omitempty"`
	CacheTo    []string          `json:"cache-to,omitempty"`
	Secrets    []string          `json:"secret,omitempty"`
	SSH        []string          `json:"ssh,omitempty"`
	Platforms  []string          `json:"platforms,omitempty"`
	Target     string            `json:"target,omitempty"`
	Pull       bool              `json:"pull,omitempty"`
	NoCache    bool              `json:"no-cache,omitempty"`
}

type bakeMetadata map[string]map[string]string

func (s *composeService) doBuildBuildkit(ctx context.Context, opts map[string]build.Options, mode string) (map[string]string, error) {
	cfg := bakeConfig{
		Groups:  map[string]bakeGroup{},
		Targets: map[string]bakeTarget{},
	}
	var group bakeGroup
	for name, options := range opts {
		cfg.Targets[name] = bakeTarget{
			Context:    options.Inputs.ContextPath,
			Dockerfile: options.Inputs.DockerfilePath,
			Args:       options.BuildArgs,
			Labels:     options.Labels,
			Tags:       options.Tags,
			// CacheFrom:  TODO
			// CacheTo:    TODO
			// Platforms:  TODO
			Target: options.Target,
			// Secrets:    TODO
			// SSH:        TODO
			Pull:    options.Pull,
			NoCache: options.NoCache,
		}
		group.Targets = append(group.Targets, name)
	}
	cfg.Groups["default"] = group

	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	metadata, err := os.CreateTemp(os.TempDir(), "compose")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "docker", "buildx", "bake", "--file", "-", "--progress", mode, "--metadata-file", metadata.Name())
	cmd.Stderr = s.stderr()
	cmd.Stdout = s.stdout()
	cmd.Stdin = bytes.NewBuffer(b)
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	b, err = os.ReadFile(metadata.Name())
	if err != nil {
		return nil, err
	}
	var md bakeMetadata
	json.Unmarshal(b, &md)

	imagesIDs := map[string]string{}
	for k, m := range md {
		imagesIDs[k] = m["containerimage.digest"]
	}
	return imagesIDs, err
}
