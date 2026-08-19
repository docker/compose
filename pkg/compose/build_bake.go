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
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/console"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/cli/cli/streams"
	"github.com/google/uuid"
	"github.com/moby/buildkit/client"
	gitutil "github.com/moby/buildkit/frontend/dockerfile/dfgitutil"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/moby/moby/client/pkg/versions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

func buildWithBake(dockerCli command.Cli) (bool, error) {
	enabled, err := dockerCli.BuildKitEnabled()
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	}

	_, err = manager.GetPlugin("buildx", dockerCli, &cobra.Command{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			logrus.Warnf("Docker Compose requires buildx plugin to be installed")
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
	Attest           []string          `json:"attest,omitempty"`
}

type bakeMetadata map[string]buildStatus

type buildStatus struct {
	Digest string `json:"containerimage.digest"`
	Image  string `json:"image.name"`
}

// bakeBuild is everything derived from the project that doBuildBake needs to
// drive `buildx bake`: the bake file definition, plus the settings that travel
// as command arguments or environment variables rather than in the file.
type bakeBuild struct {
	cfg            bakeConfig
	targetNames    map[string]string // service name -> bake target name
	expectedImages map[string]string // service name -> expected image
	localPaths     []string          // local build contexts bake needs `--allow fs.read` for
	privileged     bool
	secretsEnv     []string
}

func (s *composeService) doBuildBake(ctx context.Context, project *types.Project, serviceToBeBuild types.Services, options api.BuildOptions) (map[string]string, error) {
	eg := errgroup.Group{}
	ch := make(chan *client.SolveStatus)
	displayMode := progressui.DisplayMode(options.Progress)
	if p, ok := os.LookupEnv("BUILDKIT_PROGRESS"); ok && displayMode == progressui.AutoMode {
		displayMode = progressui.DisplayMode(p)
	}
	out := options.Out
	if out == nil {
		out = s.stdout()
	}
	display, err := progressui.NewDisplay(makeConsole(out), displayMode)
	if err != nil {
		return nil, err
	}
	eg.Go(func() error {
		_, err := display.UpdateFrom(ctx, ch)
		return err
	})

	bake := s.prepareBakeBuild(project, serviceToBeBuild, options)

	cfgJSON, err := json.MarshalIndent(bake.cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if options.Print {
		_, err = fmt.Fprintln(s.stdout(), string(cfgJSON))
		return nil, err
	}
	logrus.Debugf("bake build config:\n%s", string(cfgJSON))

	metadataFile, err := bakeMetadataPath()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.Remove(metadataFile)
	}()

	buildx, err := s.getBuildxPlugin()
	if err != nil {
		return nil, err
	}
	args := bakeArgs(bake, metadataFile, options)
	logrus.Debugf("Executing bake with args: %v", args)

	if s.dryRun {
		return s.dryRunBake(bake.cfg), nil
	}
	cmd := exec.CommandContext(ctx, buildx.Path, args...)

	err = s.prepareShellOut(ctx, types.NewMapping(os.Environ()), cmd)
	if err != nil {
		return nil, err
	}
	endpoint, cleanup, err := s.propagateDockerEndpoint()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	cmd.Env = append(cmd.Env, endpoint...)
	cmd.Env = append(cmd.Env, bake.secretsEnv...)

	cmd.Stdout = s.stdout()
	cmd.Stdin = bytes.NewBuffer(cfgJSON)
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	eg.Go(cmd.Wait)

	errMessage, err := forwardBakeStatus(pipe, ch)
	if err != nil {
		return nil, err
	}
	close(ch) // stop build progress UI

	err = eg.Wait()
	if err != nil {
		if len(errMessage) > 0 {
			return nil, errors.New(strings.Join(errMessage, "\n"))
		}
		return nil, fmt.Errorf("failed to execute bake: %w", err)
	}

	return s.collectBakeResults(ctx, metadataFile, serviceToBeBuild, bake)
}

// prepareBakeBuild translates the project's build configuration into a bake
// file definition and the side-band settings bake takes on its command line.
func (s *composeService) prepareBakeBuild(project *types.Project, serviceToBeBuild types.Services, options api.BuildOptions) *bakeBuild {
	bake := &bakeBuild{
		cfg: bakeConfig{
			Groups:  map[string]bakeGroup{},
			Targets: map[string]bakeTarget{},
		},
		targetNames:    bakeTargetNames(project),
		expectedImages: make(map[string]string, len(serviceToBeBuild)),
	}

	// project.Services lists every service (we still need their bake targets
	// defined so additional_contexts: service:xxx references can resolve),
	// but only emit "Building" progress and track expected images for
	// services we actually plan to build.
	for serviceName, service := range project.Services {
		if service.Build == nil {
			continue
		}
		buildConfig := *service.Build

		args := resolveAndMergeBuildArgs(s.getProxyConfig(), project, service, options).ToMapping()
		for k, v := range args {
			args[k] = strings.ReplaceAll(v, "${", "$${")
		}

		entitlements, privileged := bakeEntitlements(buildConfig)
		bake.privileged = bake.privileged || privileged
		bake.localPaths = append(bake.localPaths, localBuildPaths(buildConfig)...)

		image := api.GetImageNameOrDefault(service, project.Name)
		if _, ok := serviceToBeBuild[serviceName]; ok {
			s.events.On(buildingEvent(image))
			bake.expectedImages[serviceName] = image
		}

		secrets, env := toBakeSecrets(project, buildConfig.Secrets)
		bake.secretsEnv = append(bake.secretsEnv, env...)

		outputs, call := bakeOutputs(service, options)
		bake.cfg.Targets[bake.targetNames[serviceName]] = bakeTarget{
			Context:          buildConfig.Context,
			Contexts:         additionalContexts(buildConfig.AdditionalContexts, bake.targetNames),
			Dockerfile:       dockerFilePath(buildConfig.Context, buildConfig.Dockerfile),
			DockerfileInline: strings.ReplaceAll(buildConfig.DockerfileInline, "${", "$${"),
			Args:             args,
			Labels:           getImageBuildLabels(project, service),
			Tags:             append(buildConfig.Tags, image),

			CacheFrom:     buildConfig.CacheFrom,
			CacheTo:       buildConfig.CacheTo,
			NetworkMode:   buildConfig.Network,
			NoCacheFilter: buildConfig.NoCacheFilter,
			Platforms:     buildConfig.Platforms,
			Target:        buildConfig.Target,
			Secrets:       secrets,
			SSH:           toBakeSSH(append(buildConfig.SSH, options.SSHs...)),
			Pull:          buildConfig.Pull || options.Pull,
			NoCache:       buildConfig.NoCache || options.NoCache,
			ShmSize:       buildConfig.ShmSize,
			Ulimits:       toBakeUlimits(buildConfig.Ulimits),
			Entitlements:  entitlements,
			ExtraHosts:    toBakeExtraHosts(buildConfig.ExtraHosts),

			Outputs: outputs,
			Call:    call,
			Attest:  toBakeAttest(buildConfig),
		}
	}

	// create a bake group with targets for services to build
	var group bakeGroup
	for serviceName, service := range serviceToBeBuild {
		if service.Build == nil {
			continue
		}
		group.Targets = append(group.Targets, bake.targetNames[serviceName])
	}
	bake.cfg.Groups["default"] = group

	return bake
}

// bakeTargetNames produces a unique ID for each service, used as bake target.
// Replacing dots can make distinct service names collide (`a.b` vs `a_b`), so
// names are allocated in sorted service order — deterministic — and a
// colliding name gets `_` appended until unique.
func bakeTargetNames(project *types.Project) map[string]string {
	targets := make(map[string]string, len(project.Services))
	used := make(map[string]bool, len(project.Services))
	for _, serviceName := range slices.Sorted(maps.Keys(project.Services)) {
		t := strings.ReplaceAll(serviceName, ".", "_")
		for used[t] {
			t += "_"
		}
		targets[serviceName] = t
		used[t] = true
	}
	return targets
}

// bakeEntitlements returns the entitlements for a build target, and whether
// bake must be granted `security.insecure`.
func bakeEntitlements(buildConfig types.BuildConfig) ([]string, bool) {
	entitlements := buildConfig.Entitlements
	privileged := slices.Contains(buildConfig.Entitlements, "security.insecure")
	if buildConfig.Privileged {
		entitlements = append(entitlements, "security.insecure")
		privileged = true
	}
	return entitlements, privileged
}

// bakeOutputs selects the bake output type — or the lint call for `--check` —
// for a service build.
func bakeOutputs(service types.ServiceConfig, options api.BuildOptions) (outputs []string, call string) {
	push := options.Push && service.Image != ""
	switch {
	case options.Check:
		return nil, "lint"
	case len(service.Build.Platforms) > 1:
		return []string{fmt.Sprintf("type=image,push=%t", push)}, ""
	case push:
		return []string{"type=registry"}, ""
	default:
		return []string{"type=docker"}, ""
	}
}

// localBuildPaths returns the build context paths that live on the local
// filesystem — remote (git or URL) contexts need no fs.read entitlement.
func localBuildPaths(buildConfig types.BuildConfig) []string {
	paths := []string{buildConfig.Context}
	for _, path := range buildConfig.AdditionalContexts {
		paths = append(paths, path)
	}
	var local []string
	for _, path := range paths {
		if _, _, err := gitutil.ParseGitRef(path); !strings.Contains(path, "://") && err != nil {
			local = append(local, path)
		}
	}
	return local
}

// bakeMetadataPath picks a fresh temporary path for bake's --metadata-file.
// We don't use os.CreateTemp here as we need a temporary file name, but don't
// want it actually created, as bake relies on atomicwriter and this creates
// conflict during rename.
func bakeMetadataPath() (string, error) {
	tmpdir := os.TempDir()
	for {
		metadataFile := filepath.Join(tmpdir, fmt.Sprintf("compose-build-metadataFile-%s.json", uuid.New().String()))
		_, err := os.Stat(metadataFile)
		if os.IsNotExist(err) {
			return metadataFile, nil
		}
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			return "", fmt.Errorf("can't access os.tempDir %s: %w", tmpdir, pathError.Err)
		}
	}
}

// bakeArgs assembles the buildx bake command line.
func bakeArgs(bake *bakeBuild, metadataFile string, options api.BuildOptions) []string {
	args := []string{"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", metadataFile}
	// FIXME we should prompt user about this, but this is a breaking change in UX
	for _, path := range bake.localPaths {
		args = append(args, "--allow", "fs.read="+path)
	}
	if bake.privileged {
		args = append(args, "--allow", "security.insecure")
	}
	if options.SBOM != "" {
		args = append(args, "--sbom="+options.SBOM)
	}
	if options.Provenance != "" {
		args = append(args, "--provenance="+options.Provenance)
	}
	if options.Builder != "" {
		args = append(args, "--builder", options.Builder)
	}
	if options.Quiet {
		args = append(args, "--progress=quiet")
	}
	return args
}

// forwardBakeStatus reads bake's rawjson stderr stream, forwarding solve
// statuses to the progress UI channel. Lines that are not solve statuses are
// collected as error messages, to be reported if bake exits non-zero.
func forwardBakeStatus(pipe io.Reader, ch chan<- *client.SolveStatus) ([]string, error) {
	var errMessage []string
	reader := bufio.NewReader(pipe)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr == io.EOF {
			return errMessage, nil
		}
		if errors.Is(readErr, os.ErrClosed) {
			logrus.Debugf("bake stopped")
			return errMessage, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("failed to execute bake: %w", readErr)
		}
		decoder := json.NewDecoder(strings.NewReader(line))
		var status client.SolveStatus
		if err := decoder.Decode(&status); err != nil {
			errMessage = append(errMessage, strings.TrimPrefix(line, "ERROR: "))
			continue
		}
		ch <- &status
	}
}

// collectBakeResults reads bake's metadata file and maps each built image to
// its canonical digest.
func (s *composeService) collectBakeResults(ctx context.Context, metadataFile string, serviceToBeBuild types.Services, bake *bakeBuild) (map[string]string, error) {
	raw, err := os.ReadFile(metadataFile)
	if err != nil {
		return nil, err
	}
	var md bakeMetadata
	err = json.Unmarshal(raw, &md)
	if err != nil {
		return nil, err
	}

	// Bake reports the top-level attested image/index digest, which changes on
	// every build when provenance attestations are enabled — even for a fully
	// cached build (see https://github.com/docker/compose/issues/13636). For
	// images loaded into the local engine (the common non-push case), resolve
	// the canonical content digest — with the service's pinned platform when
	// set — so unchanged rebuilds don't recreate containers.
	results := map[string]string{}
	for name, service := range serviceToBeBuild {
		image := bake.expectedImages[name]
		built, ok := md[bake.targetNames[name]]
		if !ok {
			return nil, fmt.Errorf("build result not found in Bake metadata for service %s", name)
		}
		results[image] = s.canonicalBuiltDigest(ctx, image, service.Platform, built.Digest)
		s.events.On(builtEvent(image))
	}

	return results, nil
}

func (s *composeService) getBuildxPlugin() (*manager.Plugin, error) {
	buildx, err := manager.GetPlugin("buildx", s.dockerCli, &cobra.Command{})
	if err != nil {
		return nil, err
	}

	if buildx.Err != nil {
		return nil, buildx.Err
	}

	if buildx.Version == "" {
		return nil, fmt.Errorf("failed to get version of buildx")
	}

	if versions.LessThan(buildx.Version[1:], buildxMinVersion) {
		return nil, fmt.Errorf("compose build requires buildx %s or later", buildxMinVersion)
	}

	return buildx, nil
}

// makeConsole adapts the provided writer for buildkit's NewDisplay, which
// requires a [console.File] to enable the TTY rendering (it only ever writes,
// but goes through [console.ConsoleFromFile] for the ANSI capabilities).
//
// When the stream was constructed from a real file — the interactive case,
// where it wraps os.Stdout — the genuine [*os.File] is handed over: on
// Windows, containerd/console only accepts the exact os.Stdin/Stdout/Stderr
// values (identity check in newMaster), so any wrapper fails with "creating a
// console from a file is not supported on windows" and the TTY progress can
// never engage (#14086). For file-less streams the [console.File] wrapper is
// kept: the TTY rendering then works on Unix, where only the descriptor
// matters, and falls back to plain elsewhere.
func makeConsole(out io.Writer) io.Writer {
	if s, ok := out.(*streams.Out); ok {
		if f, ok := s.File(); ok {
			return f
		}
		return &_console{s}
	}
	return out
}

var _ console.File = &_console{}

type _console struct {
	*streams.Out
}

func (c _console) Read([]byte) (n int, err error) {
	return 0, errors.New("not implemented")
}

func (c _console) Close() error {
	return nil
}

func (c _console) Fd() uintptr {
	return c.FD()
}

func (c _console) Name() string {
	return "compose"
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

func toBakeSecrets(project *types.Project, secrets []types.ServiceSecretConfig) ([]string, []string) {
	var s []string
	var env []string
	for _, ref := range secrets {
		def := project.Secrets[ref.Source]
		target := ref.Target
		if target == "" {
			target = ref.Source
		}
		switch {
		case def.Environment != "":
			env = append(env, fmt.Sprintf("%s=%s", def.Environment, project.Environment[def.Environment]))
			s = append(s, fmt.Sprintf("id=%s,type=env,env=%s", target, def.Environment))
		case def.File != "":
			s = append(s, fmt.Sprintf("id=%s,type=file,src=%s", target, def.File))
		}
	}
	return s, env
}

func toBakeAttest(buildConfig types.BuildConfig) []string {
	var attests []string

	// Handle per-service provenance configuration (only from build config, not global options)
	if buildConfig.Provenance != "" {
		if buildConfig.Provenance == "true" {
			attests = append(attests, "type=provenance")
		} else if buildConfig.Provenance != "false" {
			attests = append(attests, fmt.Sprintf("type=provenance,%s", buildConfig.Provenance))
		}
	}

	// Handle per-service SBOM configuration (only from build config, not global options)
	if buildConfig.SBOM != "" {
		if buildConfig.SBOM == "true" {
			attests = append(attests, "type=sbom")
		} else if buildConfig.SBOM != "false" {
			attests = append(attests, fmt.Sprintf("type=sbom,%s", buildConfig.SBOM))
		}
	}

	return attests
}

func dockerFilePath(ctxName string, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	contextType, _ := build.DetectContextType(ctxName)
	if contextType == build.ContextTypeGit || contextType == build.ContextTypeRemote {
		return dockerfile
	}
	if strings.Contains(ctxName, "://") {
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

func (s *composeService) dryRunBake(cfg bakeConfig) map[string]string {
	bakeResponse := map[string]string{}
	for name, target := range cfg.Targets {
		dryRunUUID := fmt.Sprintf("dryRun-%x", sha1.Sum([]byte(name)))
		s.events.On(api.Resource{
			ID:     name + " ==>",
			Status: api.Done,
			Text:   fmt.Sprintf("==> writing image %s", dryRunUUID),
		})
		s.events.On(api.Resource{
			ID:     name + " ==> ==>",
			Status: api.Done,
			Text:   fmt.Sprintf(`naming to %s`, target.Tags[0]),
		})
		bakeResponse[name] = dryRunUUID
	}
	for name := range bakeResponse {
		s.events.On(builtEvent(name))
	}
	return bakeResponse
}
