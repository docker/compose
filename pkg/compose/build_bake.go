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
	"math/rand"
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
		b = "true"
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
		return false, nil
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
	ExtraHosts       map[string]string `json:"extra-hosts,omitempty"`
	Outputs          []string          `json:"output,omitempty"`
}

type bakeMetadata map[string]buildStatus

type buildStatus struct {
	Digest string `json:"containerimage.digest"`
	Image  string `json:"image.name"`
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
	var (
		group      bakeGroup
		privileged bool
		read       []string
		targets    = make(map[string]string, len(serviceToBeBuild)) // service name -> build target
	)

	// produce a unique ID for service used as bake target
	for serviceName := range project.Services {
		t := strings.ReplaceAll(serviceName, ".", "_")
		for {
			if _, ok := targets[serviceName]; !ok {
				targets[serviceName] = t
				break
			}
			t += "_"
		}
	}

	for serviceName, service := range project.Services {
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

		entitlements := build.Entitlements
		if slices.Contains(build.Entitlements, "security.insecure") {
			privileged = true
		}
		if build.Privileged {
			entitlements = append(entitlements, "security.insecure")
			privileged = true
		}

		var outputs []string
		var call string
		push := options.Push && service.Image != ""
		switch {
		case options.Check:
			call = "lint"
		case len(service.Build.Platforms) > 1:
			outputs = []string{fmt.Sprintf("type=image,push=%t", push)}
		default:
			outputs = []string{fmt.Sprintf("type=docker,load=true,push=%t", push)}
		}

		read = append(read, build.Context)
		for _, path := range build.AdditionalContexts {
			_, err := gitutil.ParseGitRef(path)
			if !strings.Contains(path, "://") && err != nil {
				read = append(read, path)
			}
		}

		target := targets[serviceName]
		cfg.Targets[target] = bakeTarget{
			Context:          build.Context,
			Contexts:         additionalContexts(build.AdditionalContexts, targets),
			Dockerfile:       dockerFilePath(build.Context, build.Dockerfile),
			DockerfileInline: strings.ReplaceAll(build.DockerfileInline, "${", "$${"),
			Args:             args,
			Labels:           build.Labels,
			Tags:             append(build.Tags, api.GetImageNameOrDefault(service, project.Name)),

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
			ExtraHosts:   toBakeExtraHosts(build.ExtraHosts),

			Outputs: outputs,
			Call:    call,
		}
	}

	// create a bake group with targets for services to build
	for serviceName, service := range serviceToBeBuild {
		if service.Build == nil {
			continue
		}
		group.Targets = append(group.Targets, targets[serviceName])
	}

	cfg.Groups["default"] = group

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}

	if options.Print {
		_, err = fmt.Fprintln(s.stdout(), string(b))
		return nil, err
	}
	logrus.Debugf("bake build config:\n%s", string(b))

	var metadataFile string
	for {
		// we don't use os.CreateTemp here as we need a temporary file name, but don't want it actually created
		// as bake relies on atomicwriter and this creates conflict during rename
		metadataFile = filepath.Join(os.TempDir(), fmt.Sprintf("compose-build-metadataFile-%d.json", rand.Int31()))
		if _, err = os.Stat(metadataFile); os.IsNotExist(err) {
			break
		}
	}
	defer func() {
		_ = os.Remove(metadataFile)
	}()

	buildx, err := manager.GetPlugin("buildx", s.dockerCli, &cobra.Command{})
	if err != nil {
		return nil, err
	}

	args := []string{"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", metadataFile}
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
	if options.Quiet {
		args = append(args, "--progress=quiet")
	}

	logrus.Debugf("Executing bake with args: %v", args)

	cmd := exec.CommandContext(ctx, buildx.Path, args...)
	// Remove DOCKER_CLI_PLUGIN... variable so buildx can detect it run standalone
	cmd.Env = filter(os.Environ(), manager.ReexecEnvvar)

	// Use docker/cli mechanism to propagate termination signal to child process
	server, err := socket.NewPluginServer(nil)
	if err == nil {
		defer server.Close() //nolint:errcheck
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

	var errMessage []string
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
				errMessage = append(errMessage, line[7:])
			} else {
				errMessage = append(errMessage, line)
			}
			continue
		}
		ch <- &status
	}
	close(ch) // stop build progress UI

	err = eg.Wait()
	if err != nil {
		if len(errMessage) > 0 {
			return nil, errors.New(strings.Join(errMessage, "\n"))
		}
		return nil, fmt.Errorf("failed to execute bake: %w", err)
	}

	b, err = os.ReadFile(metadataFile)
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
	for name := range serviceToBeBuild {
		target := targets[name]
		built, ok := md[target]
		if !ok {
			return nil, fmt.Errorf("build result not found in Bake metadata for service %s", name)
		}
		results[name] = built.Digest
		cw.Event(progress.BuiltEvent(name))
	}
	return results, nil
}

func toBakeExtraHosts(hosts types.HostsList) map[string]string {
	m := make(map[string]string)
	for k, v := range hosts {
		m[k] = strings.Join(v, ",")
	}
	return m
}

func additionalContexts(contexts types.Mapping, targets map[string]string) map[string]string {
	ac := map[string]string{}
	for k, v := range contexts {
		if target, found := strings.CutPrefix(v, types.ServicePrefix); found {
			v = "target:" + targets[target]
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
	if urlutil.IsGitURL(ctxName) {
		return dockerfile
	}
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(ctxName, dockerfile)
	}
	symlinks, err := filepath.EvalSymlinks(dockerfile)
	if err == nil {
		return symlinks
	}
	return dockerfile
}
