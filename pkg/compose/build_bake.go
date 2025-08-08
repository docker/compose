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
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
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
		if ok {
			logrus.Warnf("COMPOSE_BAKE=false is deprecated, support for internal compose builder will be removed in next release")
		}
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
		if errdefs.IsNotFound(err) {
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
	if options.Progress == progress.ModeAuto {
		options.Progress = os.Getenv("BUILDKIT_PROGRESS")
	}
	displayMode := progressui.DisplayMode(options.Progress)
	out := options.Out
	if out == nil {
		if !s.dockerCli.Out().IsTerminal() {
			displayMode = progressui.PlainMode
		}
		out = os.Stdout // should be s.dockerCli.Out(), but NewDisplay require access to the underlying *File
	}
	display, err := progressui.NewDisplay(out, displayMode)
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
		group          bakeGroup
		privileged     bool
		read           []string
		expectedImages = make(map[string]string, len(serviceToBeBuild)) // service name -> expected image
		targets        = make(map[string]string, len(serviceToBeBuild)) // service name -> build target
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
		labels := getImageBuildLabels(project, service)

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
			if push {
				outputs = []string{"type=registry"}
			} else {
				outputs = []string{"type=docker"}
			}
		}

		read = append(read, build.Context)
		for _, path := range build.AdditionalContexts {
			_, err := gitutil.ParseGitRef(path)
			if !strings.Contains(path, "://") && err != nil {
				read = append(read, path)
			}
		}

		image := api.GetImageNameOrDefault(service, project.Name)
		expectedImages[serviceName] = image

		pull := service.Build.Pull || options.Pull
		noCache := service.Build.NoCache || options.NoCache

		target := targets[serviceName]
		cfg.Targets[target] = bakeTarget{
			Context:          build.Context,
			Contexts:         additionalContexts(build.AdditionalContexts, targets),
			Dockerfile:       dockerFilePath(build.Context, build.Dockerfile),
			DockerfileInline: strings.ReplaceAll(build.DockerfileInline, "${", "$${"),
			Args:             args,
			Labels:           labels,
			Tags:             append(build.Tags, image),

			CacheFrom:    build.CacheFrom,
			CacheTo:      build.CacheTo,
			NetworkMode:  build.Network,
			Platforms:    build.Platforms,
			Target:       build.Target,
			Secrets:      toBakeSecrets(project, build.Secrets),
			SSH:          toBakeSSH(append(build.SSH, options.SSHs...)),
			Pull:         pull,
			NoCache:      noCache,
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

	if s.dryRun {
		return dryRunBake(ctx, cfg), nil
	}
	cmd := exec.CommandContext(ctx, buildx.Path, args...)

	err = s.prepareShellOut(ctx, project.Environment, cmd)
	if err != nil {
		return nil, err
	}

	cmd.Stdout = s.stdout()
	cmd.Stdin = bytes.NewBuffer(b)
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	var errMessage []string
	reader := bufio.NewReader(pipe)

	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	eg.Go(cmd.Wait)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				break
			} else {
				return nil, fmt.Errorf("failed to execute bake: %w", readErr)
			}
		}
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
		image := expectedImages[name]
		target := targets[name]
		built, ok := md[target]
		if !ok {
			return nil, fmt.Errorf("build result not found in Bake metadata for service %s", name)
		}
		results[image] = built.Digest
		cw.Event(progress.BuiltEvent(image))
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

func dockerFilePath(ctxName, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	if urlutil.IsGitURL(ctxName) {
		return dockerfile
	}
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(ctxName, dockerfile)
	}
	dir := filepath.Dir(dockerfile)
	symlinks, err := filepath.EvalSymlinks(dir)
	if err == nil {
		return filepath.Join(symlinks, filepath.Base(dockerfile))
	}
	return dockerfile
}

func dryRunBake(ctx context.Context, cfg bakeConfig) map[string]string {
	w := progress.ContextWriter(ctx)
	bakeResponse := map[string]string{}
	for name, target := range cfg.Targets {
		dryRunUUID := fmt.Sprintf("dryRun-%x", sha1.Sum([]byte(name)))
		displayDryRunBuildEvent(w, name, dryRunUUID, target.Tags[0])
		bakeResponse[name] = dryRunUUID
	}
	for name := range bakeResponse {
		w.Event(progress.BuiltEvent(name))
	}
	return bakeResponse
}

func displayDryRunBuildEvent(w progress.Writer, name, dryRunUUID, tag string) {
	w.Event(progress.Event{
		ID:     name + " ==>",
		Status: progress.Done,
		Text:   fmt.Sprintf("==> writing image %s", dryRunUUID),
	})
	w.Event(progress.Event{
		ID:     name + " ==> ==>",
		Status: progress.Done,
		Text:   fmt.Sprintf(`naming to %s`, tag),
	})
}
