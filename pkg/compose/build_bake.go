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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/socket"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/builder/remotecontext/urlutil"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/gitutil"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
)

func buildWithBake(dockerCli command.Cli) (bool, error) {
	b, ok := os.LookupEnv("COMPOSE_BAKE")
	if !ok {
		if dockerCli.ConfigFile().Plugins["compose"]["build"] == "bake" {
			b, ok = "true", true
		}
	}
	if !ok {
		return false, nil
	}
	bake, err := strconv.ParseBool(b)
	if err != nil {
		return false, err
	}
	if !bake {
		return false, nil
	}

	enabled, err := dockerCli.BuildKitEnabled()
	if err != nil {
		return false, err
	}
	if !enabled {
		logrus.Warnf("Docker Compose is configured to build using Bake, but buildkit isn't enabled")
	}

	_, err = manager.GetPlugin("buildx", dockerCli, &cobra.Command{})
	if err != nil {
		if manager.IsNotFound(err) {
			logrus.Warnf("Docker Compose is configured to build using Bake, but buildx isn't installed")
			return false, nil
		}
		return false, err
	}
	return true, err
}

// We _could_ use bake.* types from github.com/docker/buildx but long term plan is to remove buildx as a dependency
type bakeConfig struct {
	Groups  map[string]bakeGroup  `json:"group"`
	Targets map[string]bakeTarget `json:"target"`
}

type bakeGroup struct {
	Targets []string `json:"targets"`
}

type bakeTarget struct {
	Context          string            `json:"context,omitempty"`
	Contexts         map[string]string `json:"contexts,omitempty"`
	Dockerfile       string            `json:"dockerfile,omitempty"`
	DockerfileInline string            `json:"dockerfile-inline,omitempty"`
	Args             map[string]string `json:"args,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	CacheFrom        []string          `json:"cache-from,omitempty"`
	CacheTo          []string          `json:"cache-to,omitempty"`
	Target           string            `json:"target,omitempty"`
	Secrets          []string          `json:"secret,omitempty"`
	SSH              []string          `json:"ssh,omitempty"`
	Platforms        []string          `json:"platforms,omitempty"`
	Pull             bool              `json:"pull,omitempty"`
	NoCache          bool              `json:"no-cache,omitempty"`
	NetworkMode      string            `json:"network,omitempty"`
	NoCacheFilter    []string          `json:"no-cache-filter,omitempty"`
	ShmSize          types.UnitBytes   `json:"shm-size,omitempty"`
	Ulimits          []string          `json:"ulimits,omitempty"`
	Call             string            `json:"call,omitempty"`
	Entitlements     []string          `json:"entitlements,omitempty"`
	Outputs          []string          `json:"output,omitempty"`
}

type bakeMetadata map[string]buildStatus

type buildStatus struct {
	Digest string `json:"containerimage.digest"`
}

func (s *composeService) doBuildBake(ctx context.Context, project *types.Project, serviceToBeBuild types.Services, options api.BuildOptions) (map[string]string, error) { //nolint:gocyclo
	eg := errgroup.Group{}
	ch := make(chan *client.SolveStatus)
	display, err := progressui.NewDisplay(os.Stdout, progressui.DisplayMode(options.Progress))
	if err != nil {
		return nil, err
	}
	eg.Go(func() error {
		_, err := display.UpdateFrom(ctx, ch)
		return err
	})

	cfg := bakeConfig{
		Groups:  map[string]bakeGroup{},
		Targets: map[string]bakeTarget{},
	}
	var group bakeGroup
	var privileged bool
	var read []string

	for serviceName, service := range serviceToBeBuild {
		if service.Build == nil {
			continue
		}
		build := *service.Build

		args := types.Mapping{}
		for k, v := range resolveAndMergeBuildArgs(s.dockerCli, project, service, options) {
			if v == nil {
				continue
			}
			args[k] = *v
		}

		image := api.GetImageNameOrDefault(service, project.Name)

		entitlements := build.Entitlements
		if slices.Contains(build.Entitlements, "security.insecure") {
			privileged = true
		}
		if build.Privileged {
			entitlements = append(entitlements, "security.insecure")
			privileged = true
		}

		var output string
		push := options.Push && service.Image != ""
		if len(service.Build.Platforms) > 1 {
			output = fmt.Sprintf("type=image,push=%t", push)
		} else {
			output = fmt.Sprintf("type=docker,load=true,push=%t", push)
		}

		read = append(read, build.Context)
		for _, path := range build.AdditionalContexts {
			_, err := gitutil.ParseGitRef(path)
			if !strings.Contains(path, "://") && err != nil {
				read = append(read, path)
			}
		}

		cfg.Targets[serviceName] = bakeTarget{
			Context:          build.Context,
			Contexts:         additionalContexts(build.AdditionalContexts),
			Dockerfile:       dockerFilePath(build.Context, build.Dockerfile),
			DockerfileInline: build.DockerfileInline,
			Args:             args,
			Labels:           build.Labels,
			Tags:             append(build.Tags, image),

			CacheFrom: build.CacheFrom,
			// CacheTo:    TODO
			Platforms:    build.Platforms,
			Target:       build.Target,
			Secrets:      toBakeSecrets(project, build.Secrets),
			SSH:          toBakeSSH(append(build.SSH, options.SSHs...)),
			Pull:         options.Pull,
			NoCache:      options.NoCache,
			ShmSize:      build.ShmSize,
			Ulimits:      toBakeUlimits(build.Ulimits),
			Entitlements: entitlements,
			Outputs:      []string{output},
		}
		group.Targets = append(group.Targets, serviceName)
	}

	cfg.Groups["default"] = group

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}

	logrus.Debugf("bake build config:\n%s", string(b))

	metadata, err := os.CreateTemp(os.TempDir(), "compose")
	if err != nil {
		return nil, err
	}

	buildx, err := manager.GetPlugin("buildx", s.dockerCli, &cobra.Command{})
	if err != nil {
		return nil, err
	}

	args := []string{"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", metadata.Name()}
	mustAllow := buildx.Version != "" && versions.GreaterThanOrEqualTo(buildx.Version[1:], "0.17.0")
	if mustAllow {
		// FIXME we should prompt user about this, but this is a breaking change in UX
		for _, path := range read {
			args = append(args, "--allow", "fs.read="+path)
		}
		if privileged {
			args = append(args, "--allow", "security.insecure")
		}
	}

	if options.Builder != "" {
		args = append(args, "--builder", options.Builder)
	}

	logrus.Debugf("Executing bake with args: %v", args)

	cmd := exec.CommandContext(ctx, buildx.Path, args...)
	// Remove DOCKER_CLI_PLUGIN... variable so buildx can detect it run standalone
	cmd.Env = filter(os.Environ(), manager.ReexecEnvvar)

	// Use docker/cli mechanism to propagate termination signal to child process
	server, err := socket.NewPluginServer(nil)
	if err != nil {
		defer server.Close() //nolint:errcheck
		cmd.Cancel = server.Close
		cmd.Env = replace(cmd.Env, socket.EnvKey, server.Addr().String())
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_CONTEXT=%s", s.dockerCli.CurrentContext()))

	// propagate opentelemetry context to child process, see https://github.com/open-telemetry/oteps/blob/main/text/0258-env-context-baggage-carriers.md
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, &carrier)
	cmd.Env = append(cmd.Env, types.Mapping(carrier).Values()...)

	cmd.Stdout = s.stdout()
	cmd.Stdin = bytes.NewBuffer(b)
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	var errMessage string
	scanner := bufio.NewScanner(pipe)
	scanner.Split(bufio.ScanLines)

	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	eg.Go(cmd.Wait)
	for scanner.Scan() {
		line := scanner.Text()
		decoder := json.NewDecoder(strings.NewReader(line))
		var status client.SolveStatus
		err := decoder.Decode(&status)
		if err != nil {
			if strings.HasPrefix(line, "ERROR: ") {
				errMessage = line[7:]
			}
			continue
		}
		ch <- &status
	}
	close(ch) // stop build progress UI

	err = eg.Wait()
	if err != nil {
		if errMessage != "" {
			return nil, errors.New(errMessage)
		}
		return nil, fmt.Errorf("failed to execute bake: %w", err)
	}

	b, err = os.ReadFile(metadata.Name())
	if err != nil {
		return nil, err
	}

	var md bakeMetadata
	err = json.Unmarshal(b, &md)
	if err != nil {
		return nil, err
	}

	cw := progress.ContextWriter(ctx)
	results := map[string]string{}
	for name, m := range md {
		results[name] = m.Digest
		cw.Event(progress.BuiltEvent(name))
	}
	return results, nil
}

func additionalContexts(contexts types.Mapping) map[string]string {
	ac := map[string]string{}
	for k, v := range contexts {
		if target, found := strings.CutPrefix(v, types.ServicePrefix); found {
			v = "target:" + target
		}
		ac[k] = v
	}
	return ac
}

func toBakeUlimits(ulimits map[string]*types.UlimitsConfig) []string {
	s := []string{}
	for u, l := range ulimits {
		if l.Single > 0 {
			s = append(s, fmt.Sprintf("%s=%d", u, l.Single))
		} else {
			s = append(s, fmt.Sprintf("%s=%d:%d", u, l.Soft, l.Hard))
		}
	}
	return s
}

func toBakeSSH(ssh types.SSHConfig) []string {
	var s []string
	for _, key := range ssh {
		s = append(s, fmt.Sprintf("%s=%s", key.ID, key.Path))
	}
	return s
}

func toBakeSecrets(project *types.Project, secrets []types.ServiceSecretConfig) []string {
	var s []string
	for _, ref := range secrets {
		def := project.Secrets[ref.Source]
		target := ref.Target
		if target == "" {
			target = ref.Source
		}
		switch {
		case def.Environment != "":
			s = append(s, fmt.Sprintf("id=%s,type=env,env=%s", target, def.Environment))
		case def.File != "":
			s = append(s, fmt.Sprintf("id=%s,type=file,src=%s", target, def.File))
		}
	}
	return s
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

func replace(environ []string, variable, value string) []string {
	filtered := filter(environ, variable)
	return append(filtered, fmt.Sprintf("%s=%s", variable, value))
}

func dockerFilePath(ctxName string, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	if urlutil.IsGitURL(ctxName) || filepath.IsAbs(dockerfile) {
		return dockerfile
	}
	return filepath.Join(ctxName, dockerfile)
}
