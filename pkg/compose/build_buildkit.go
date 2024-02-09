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
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/buildx/build"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/moby/buildkit/client"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// TODO use bake.* types
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

type bakeMetadata map[string]buildStatus

type buildStatus struct {
	Digest string `json:"containerimage.digest"`
}

func (s *composeService) doBuildBuildkit(ctx context.Context, options build.Options, ch chan *client.SolveStatus) (string, error) {
	cfg := bakeConfig{
		Groups:  map[string]bakeGroup{},
		Targets: map[string]bakeTarget{},
	}
	var group bakeGroup
	cfg.Targets["build"] = bakeTarget{
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
	group.Targets = append(group.Targets, "build")
	cfg.Groups["default"] = group

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	metadata, err := os.CreateTemp(os.TempDir(), "compose")
	if err != nil {
		return "", err
	}

	buildx, err := manager.GetPlugin("buildx", s.dockerCli, &cobra.Command{})
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, buildx.Path, "bake", "--file", "-", "--progress", "rawjson", "--metadata-file", metadata.Name())
	// Remove DOCKER_CLI_PLUGIN... variable so buildx can detect it run standalone
	cmd.Env = filter(os.Environ(), manager.ReexecEnvvar)
	// TODO propagate opentelemetry context to child process, see https://github.com/open-telemetry/opentelemetry-specification/issues/740
	cmd.Stdout = s.stdout()
	cmd.Stdin = bytes.NewBuffer(b)
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	err = cmd.Start()
	if err != nil {
		return "", err
	}
	eg := errgroup.Group{}
	eg.Go(func() error {
		for {
			decoder := json.NewDecoder(pipe)
			var s client.SolveStatus
			err := decoder.Decode(&s)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				// bake displays build details at the end of a build, which isn't a json SolveStatus
				continue
			}
			ch <- &s
		}
	})
	eg.Go(cmd.Wait)
	err = eg.Wait()
	if err != nil {
		return "", err
	}

	b, err = os.ReadFile(metadata.Name())
	if err != nil {
		return "", err
	}

	var md bakeMetadata
	err = json.Unmarshal(b, &md)
	if err != nil {
		return "", err
	}

	for _, m := range md {
		return m.Digest, nil
	}
	return "", errors.New("failed to retrieve image digest from bake metadata")
}

func filter(environ []string, variable string) []string {
	prefix := variable + "="
	filtered := make([]string, 0, len(environ))
	for _, val := range environ {
		if !strings.HasPrefix(val, prefix) {
			filtered = append(filtered, val)
		}
	}
	return filtered
}
