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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
)

// SandboxIsolation is the (PoC) isolation value routing a service to a Docker
// Sandbox (microVM managed by sandboxd via the sbx CLI) instead of the engine.
//
// The service image is exported from the engine into the sandbox runtime's
// image store, a sandbox is created from it with all egress denied, its
// published ports are bound on the host loopback, and the service process
// (entrypoint/command from the compose model or the image config) is started
// with sbx exec. Other services resolve its compose service name via an
// injected extra_hosts entry pointing at host-gateway, and reach it on the
// published port (e.g. http://api:5734).
const SandboxIsolation = "sandbox"

// sandboxServices returns the services declaring isolation: sandbox.
func sandboxServices(project *types.Project) []types.ServiceConfig {
	var out []types.ServiceConfig
	for _, service := range project.Services {
		if service.Isolation == SandboxIsolation {
			out = append(out, service)
		}
	}
	return out
}

// sandboxNameFor derives the sbx sandbox name for a service. sbx names accept
// letters, digits, hyphens and periods only.
func sandboxNameFor(projectName, serviceName string) string {
	name := projectName + "-" + serviceName
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		default:
			return '-'
		}
	}, name)
}

// prepareSandboxServices starts every isolation:sandbox service in a Docker
// Sandbox and returns the project narrowed to the engine-bound services, with
// extra_hosts entries injected so the sandboxed services stay reachable under
// their compose service name.
func (s *composeService) prepareSandboxServices(ctx context.Context, project *types.Project) (*types.Project, error) {
	sandboxed := sandboxServices(project)
	if len(sandboxed) == 0 {
		return project, nil
	}
	if _, err := exec.LookPath("sbx"); err != nil {
		return nil, fmt.Errorf("service with isolation %q requires the sbx CLI (Docker Sandboxes): %w", SandboxIsolation, err)
	}

	names := make([]string, 0, len(sandboxed))
	for _, service := range sandboxed {
		if err := s.upSandbox(ctx, project, service); err != nil {
			return nil, fmt.Errorf("starting service %q in a sandbox: %w", service.Name, err)
		}
		names = append(names, service.Name)
	}

	// Engine-bound services resolve a sandboxed service's name to the host
	// gateway, where the sandbox port is published on the loopback.
	for name, service := range project.Services {
		if service.Isolation == SandboxIsolation {
			continue
		}
		if service.ExtraHosts == nil {
			service.ExtraHosts = types.HostsList{}
		}
		for _, sbxName := range names {
			if _, declared := service.ExtraHosts[sbxName]; !declared {
				service.ExtraHosts[sbxName] = []string{"host-gateway"}
			}
		}
		project.Services[name] = service
	}

	return project.WithServicesDisabled(names...), nil
}

// upSandbox runs one service in a Docker Sandbox: image exported from the
// engine and loaded in the sandbox runtime, sandbox created with egress
// denied and ports published, service process started detached.
func (s *composeService) upSandbox(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	name := sandboxNameFor(project.Name, service.Name)
	eventName := "Sandbox " + name

	if s.sandboxExists(ctx, name) {
		s.events.On(newEvent(eventName, api.Done, "Running"))
		return nil
	}

	s.events.On(creatingEvent(eventName))

	image := api.GetImageNameOrDefault(service, project.Name)
	inspect, err := s.apiClient().ImageInspect(ctx, image)
	if err != nil {
		return fmt.Errorf("image %q must be present in the engine store (build or pull it first): %w", image, err)
	}

	if err := s.loadSandboxImage(ctx, image); err != nil {
		return err
	}

	createArgs := []string{
		"create", "shell", project.WorkingDir,
		"--template", image,
		"--name", name,
		"--pull", "missing",
		"--deny-network", "**",
		"-q",
	}
	for _, spec := range sandboxPortSpecs(service) {
		createArgs = append(createArgs, "-p", spec)
	}
	if err := runSbx(ctx, createArgs...); err != nil {
		return err
	}

	execArgs := []string{"exec"}
	workingDir := service.WorkingDir
	if workingDir == "" {
		workingDir = inspect.Config.WorkingDir
	}
	if workingDir != "" {
		execArgs = append(execArgs, "-w", workingDir)
	}
	// the exec'd process does not inherit the image config: carry its
	// environment, then the service-level overrides
	for _, kv := range inspect.Config.Env {
		execArgs = append(execArgs, "-e", kv)
	}
	for k, v := range service.Environment {
		if v == nil {
			continue
		}
		execArgs = append(execArgs, "-e", k+"="+*v)
	}
	command := sandboxCommand(service, inspect.Config.Entrypoint, inspect.Config.Cmd)
	if len(command) == 0 {
		return fmt.Errorf("service %q declares no command and its image has no entrypoint or cmd", service.Name)
	}
	// `sbx exec -d` stays attached to the service process; background it
	// through the shell instead, so the exec returns once the process is
	// spawned (its output lands in the sandbox at /tmp/compose-service.log)
	var quoted []string
	for _, arg := range command {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	execArgs = append(execArgs, name,
		"sh", "-c", "nohup "+strings.Join(quoted, " ")+" >/tmp/compose-service.log 2>&1 &")
	if err := runSbx(ctx, execArgs...); err != nil {
		return err
	}

	s.events.On(newEvent(eventName, api.Done, api.StatusStarted))
	return nil
}

// removeSandboxServices tears down the sandboxes of isolation:sandbox
// services, best effort.
func (s *composeService) removeSandboxServices(ctx context.Context, project *types.Project) {
	if project == nil {
		return
	}
	if _, err := exec.LookPath("sbx"); err != nil {
		return
	}
	all := project.AllServices()
	for _, service := range all {
		if service.Isolation != SandboxIsolation {
			continue
		}
		name := sandboxNameFor(project.Name, service.Name)
		if !s.sandboxExists(ctx, name) {
			continue
		}
		eventName := "Sandbox " + name
		s.events.On(removingEvent(eventName))
		if err := runSbx(ctx, "rm", "-f", name); err != nil {
			s.events.On(errorEventf(eventName, "failed to remove sandbox: %v", err))
			continue
		}
		s.events.On(removedEvent(eventName))
	}
}

// sandboxPortSpecs translates the service's port mappings into sbx publish
// specs, bound on the host loopback. Engine-bound services reach the
// sandboxed service through host-gateway on the published port.
func sandboxPortSpecs(service types.ServiceConfig) []string {
	var specs []string
	seen := map[string]bool{}
	add := func(spec string) {
		if !seen[spec] {
			seen[spec] = true
			specs = append(specs, spec)
		}
	}
	for _, port := range service.Ports {
		target := fmt.Sprintf("%d", port.Target)
		if port.Published != "" {
			spec := port.Published + ":" + target
			if port.HostIP != "" {
				spec = port.HostIP + ":" + spec
			}
			add(spec)
		} else {
			add(target)
		}
	}
	return specs
}

// sandboxCommand resolves the process to run in the sandbox: compose-level
// entrypoint/command override their image config counterparts, exactly as the
// engine would.
func sandboxCommand(service types.ServiceConfig, imageEntrypoint, imageCmd []string) []string {
	entrypoint := []string(service.Entrypoint)
	if entrypoint == nil {
		entrypoint = imageEntrypoint
	}
	command := []string(service.Command)
	if command == nil {
		command = imageCmd
	}
	return append(slices.Clone(entrypoint), command...)
}

// sandboxExists reports whether a sandbox with this name is known to sbx.
func (s *composeService) sandboxExists(ctx context.Context, name string) bool {
	out, err := exec.CommandContext(ctx, "sbx", "ls", "-q").Output()
	if err != nil {
		return false
	}
	return slices.Contains(strings.Fields(string(out)), name)
}

// loadSandboxImage exports an image from the engine store and loads it into
// the sandbox runtime's image store.
func (s *composeService) loadSandboxImage(ctx context.Context, image string) error {
	save, err := s.apiClient().ImageSave(ctx, []string{image})
	if err != nil {
		return fmt.Errorf("exporting image %q: %w", image, err)
	}
	defer save.Close() //nolint:errcheck

	tar, err := os.CreateTemp("", "compose-sandbox-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(tar.Name()) //nolint:errcheck
	if _, err := io.Copy(tar, save); err != nil {
		tar.Close() //nolint:errcheck
		return fmt.Errorf("writing image tar: %w", err)
	}
	if err := tar.Close(); err != nil {
		return err
	}
	return runSbx(ctx, "template", "load", tar.Name())
}

// runSbx runs an sbx CLI command, surfacing its output on failure.
func runSbx(ctx context.Context, args ...string) error {
	logrus.Debugf("running: sbx %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "sbx", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sbx %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// sandboxExec forwards a compose exec targeting a sandboxed service to
// sbx exec, with full terminal passthrough.
func (s *composeService) sandboxExec(ctx context.Context, name string, options api.RunOptions) (int, error) {
	args := []string{"exec"}
	if options.Interactive {
		args = append(args, "-i")
	}
	if options.Tty {
		args = append(args, "-t")
	}
	if options.Detach {
		args = append(args, "-d")
	}
	if options.User != "" {
		args = append(args, "-u", options.User)
	}
	if options.Privileged {
		args = append(args, "--privileged")
	}
	if options.WorkingDir != "" {
		args = append(args, "-w", options.WorkingDir)
	}
	for _, kv := range options.Environment {
		args = append(args, "-e", kv)
	}
	args = append(args, name)
	args = append(args, options.Command...)

	logrus.Debugf("forwarding exec to: sbx %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "sbx", args...)
	// hand the real terminal file descriptors over so interactive TTY
	// sessions (raw mode, resize) work as with a plain sbx exec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return code, cli.StatusError{StatusCode: code, Status: err.Error()}
	}
	return 0, err
}
