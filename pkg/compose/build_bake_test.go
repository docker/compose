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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/buildkit/client"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestLocalBuildPaths(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, "app")
	dockerfileDir := filepath.Join(root, "docker")
	assert.NilError(t, os.MkdirAll(contextDir, 0o755))
	assert.NilError(t, os.MkdirAll(dockerfileDir, 0o755))
	resolvedDir, err := filepath.EvalSymlinks(dockerfileDir)
	assert.NilError(t, err)
	secretFile := filepath.Join(root, "credentials")
	project := &types.Project{Secrets: types.Secrets{
		"file":   {File: secretFile},
		"env":    {Environment: "TOKEN"},
		"unused": {File: filepath.Join(root, "unused")},
	}}

	for _, dockerfile := range []string{filepath.Join("..", "docker", "Dockerfile"), filepath.Join(dockerfileDir, "Dockerfile")} {
		t.Run(dockerfile, func(t *testing.T) {
			config := types.BuildConfig{
				Context:            contextDir,
				Dockerfile:         dockerfile,
				AdditionalContexts: map[string]string{"remote": "https://example.com/context.git"},
				Secrets: []types.ServiceSecretConfig{
					{Source: "file", Target: "renamed"},
					{Source: "env"},
				},
			}
			assert.DeepEqual(t, localBuildPaths(project, config), []string{contextDir, filepath.Join(resolvedDir, "Dockerfile"), secretFile})
		})
	}

	for _, remote := range []string{"https://example.com/context.git", "git@github.com:docker/compose.git"} {
		t.Run(remote, func(t *testing.T) {
			config := types.BuildConfig{Context: remote, Dockerfile: "Dockerfile", Secrets: []types.ServiceSecretConfig{{Source: "file"}}}
			assert.DeepEqual(t, localBuildPaths(project, config), []string{secretFile})
		})
	}

	t.Run("remote context with local Dockerfile", func(t *testing.T) {
		config := types.BuildConfig{Context: "https://example.com/context.git", Dockerfile: filepath.Join(dockerfileDir, "Dockerfile")}
		assert.DeepEqual(t, localBuildPaths(project, config), []string{config.Dockerfile})
	})

	t.Run("inline Dockerfile", func(t *testing.T) {
		config := types.BuildConfig{Context: contextDir, DockerfileInline: "FROM scratch"}
		assert.DeepEqual(t, localBuildPaths(project, config), []string{contextDir})
	})

	t.Run("additional local context", func(t *testing.T) {
		config := types.BuildConfig{Context: contextDir, AdditionalContexts: map[string]string{"local": dockerfileDir}}
		assert.DeepEqual(t, localBuildPaths(project, config), []string{contextDir, dockerfileDir})
	})
}

func TestBakeEntitlementGrants(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config types.BuildConfig
		want   []string
	}{
		{
			name: "none",
		},
		{
			name:   "explicit host network",
			config: types.BuildConfig{Entitlements: []string{"network.host"}},
			want:   []string{"network.host"},
		},
		{
			name:   "host network mode",
			config: types.BuildConfig{Network: "host"},
			want:   []string{"network.host"},
		},
		{
			name:   "privileged",
			config: types.BuildConfig{Privileged: true},
			want:   []string{"security.insecure"},
		},
		{
			name:   "explicit insecure",
			config: types.BuildConfig{Entitlements: []string{"security.insecure"}},
			want:   []string{"security.insecure"},
		},
		{
			name:   "already requested",
			config: types.BuildConfig{Network: "host", Privileged: true, Entitlements: []string{"network.host", "security.insecure"}},
			want:   []string{"network.host", "security.insecure"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := slices.Clone(tc.config.Entitlements)
			entitlements := bakeEntitlements(tc.config)
			assert.DeepEqual(t, entitlements, tc.want)
			assert.DeepEqual(t, tc.config.Entitlements, original)
			args := bakeArgs(&bakeBuild{entitlements: append(entitlements, entitlements...)}, "metadata.json", api.BuildOptions{})
			want := []string{"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", "metadata.json"}
			for _, entitlement := range tc.want {
				want = append(want, "--allow", entitlement)
			}
			assert.DeepEqual(t, args, want)
		})
	}
}

func TestBakeReadGrants(t *testing.T) {
	bake := &bakeBuild{localPaths: []string{"/project", "/outside/Dockerfile", "/secrets/token", "/project"}}
	assert.DeepEqual(t, bakeArgs(bake, "metadata.json", api.BuildOptions{}), []string{
		"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", "metadata.json",
		"--allow", "fs.read=/outside/Dockerfile", "--allow", "fs.read=/project", "--allow", "fs.read=/secrets/token",
	})
}

func TestPrepareBakeBuildGrants(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, "app")
	secretFile := filepath.Join(root, "token")
	dockerfile := filepath.Join(root, "docker", "Dockerfile")
	project := &types.Project{
		Name: "grants",
		Services: types.Services{"app": {Name: "app", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{
			Context: contextDir, Dockerfile: dockerfile,
			Secrets:      []types.ServiceSecretConfig{{Source: "token"}},
			Entitlements: []string{"network.host"},
		}}}},
		Secrets: types.Secrets{"token": {File: secretFile}},
	}
	s := &composeService{proxyConfig: map[string]string{}, events: &ignore{}}
	bake := s.prepareBakeBuild(project, project.Services, api.BuildOptions{})
	assert.DeepEqual(t, bake.cfg.Targets["app"].Entitlements, []string{"network.host"})
	assert.DeepEqual(t, bake.cfg.Targets["app"].Secrets, []string{"id=token,type=file,src=" + secretFile})
	args := bakeArgs(bake, "metadata.json", api.BuildOptions{})
	for _, grant := range []string{"fs.read=" + contextDir, "fs.read=" + dockerfile, "fs.read=" + secretFile, "network.host"} {
		index := slices.Index(args, grant)
		assert.Assert(t, index > 0, "missing grant %s in %v", grant, args)
		assert.Equal(t, args[index-1], "--allow")
	}
	assert.Assert(t, !slices.Contains(args, "fs.read="+filepath.Dir(dockerfile)), "must not grant the Dockerfile's parent directory")
}

func TestPrepareBakeCWDGrants(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, tc := range []struct {
		name    string
		context string
		want    string
	}{
		{
			name:    "relative context",
			context: "cwd://app",
			want:    filepath.Join("docker", "Dockerfile"),
		},
		{
			name:    "absolute context",
			context: "cwd://" + filepath.Join(root, "app"),
			want:    filepath.Join(root, "docker", "Dockerfile"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dockerfile := filepath.Join("..", "docker", "Dockerfile")
			project := &types.Project{
				Name: "cwd-grants",
				Services: types.Services{"app": {Name: "app", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{
					Context: tc.context, Dockerfile: dockerfile,
				}}}},
			}
			s := &composeService{proxyConfig: map[string]string{}, events: &ignore{}}
			bake := s.prepareBakeBuild(project, project.Services, api.BuildOptions{})
			assert.Equal(t, bake.cfg.Targets["app"].Dockerfile, dockerfile)
			assert.DeepEqual(t, bake.localPaths, []string{strings.TrimPrefix(tc.context, "cwd://"), tc.want})
			assert.Assert(t, slices.Contains(bakeArgs(bake, "metadata.json", api.BuildOptions{}), "fs.read="+tc.want))
		})
	}
}

func TestPrepareBakeSSHGrants(t *testing.T) {
	for _, tc := range []struct {
		name       string
		context    string
		ssh        types.SSHConfig
		cliSSH     []types.SSHKey
		wantSSH    []string
		wantGrants []string
	}{
		{
			name: "none",
		},
		{
			name:       "configured key",
			ssh:        types.SSHConfig{{ID: "key", Path: "/keys/id"}},
			wantSSH:    []string{"key=/keys/id"},
			wantGrants: []string{"fs.read=/keys/id"},
		},
		{
			name:       "configured agent",
			ssh:        types.SSHConfig{{ID: "default"}},
			wantSSH:    []string{"default"},
			wantGrants: []string{"ssh"},
		},
		{
			name:       "CLI socket",
			cliSSH:     []types.SSHKey{{ID: "default", Path: "/agent/socket"}},
			wantSSH:    []string{"default=/agent/socket"},
			wantGrants: []string{"fs.read=/agent/socket"},
		},
		{
			name:       "CLI agent",
			cliSSH:     []types.SSHKey{{ID: "default"}},
			wantSSH:    []string{"default"},
			wantGrants: []string{"ssh"},
		},
		{
			name:       "multiple key paths",
			cliSSH:     []types.SSHKey{{ID: "keys", Path: "/keys/one,/keys/two"}},
			wantSSH:    []string{"keys=/keys/one,/keys/two"},
			wantGrants: []string{"fs.read=/keys/one", "fs.read=/keys/two"},
		},
		{
			name:       "both sources",
			ssh:        types.SSHConfig{{ID: "key", Path: "/keys/id"}},
			cliSSH:     []types.SSHKey{{ID: "default"}},
			wantSSH:    []string{"key=/keys/id", "default"},
			wantGrants: []string{"fs.read=/keys/id", "ssh"},
		},
		{
			name:       "implicit Git agent",
			context:    "git@github.com:docker/compose.git",
			wantGrants: []string{"ssh"},
		},
		{
			name:       "implicit SSH URL agent",
			context:    "ssh://git@example.com/project.git",
			wantGrants: []string{"ssh"},
		},
		{
			name:    "HTTPS Git context",
			context: "https://example.com/project.git",
		},
		{
			name:       "explicit Git key",
			context:    "git@github.com:docker/compose.git",
			ssh:        types.SSHConfig{{ID: "default", Path: "/keys/id"}},
			wantSSH:    []string{"default=/keys/id"},
			wantGrants: []string{"fs.read=/keys/id"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contextPath := tc.context
			if contextPath == "" {
				contextPath = "https://example.com/project.git"
			}
			project := &types.Project{Name: "ssh", Services: types.Services{"app": {
				Name: "app", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: contextPath, SSH: tc.ssh}},
			}}}
			s := &composeService{proxyConfig: map[string]string{}, events: &ignore{}}
			bake := s.prepareBakeBuild(project, project.Services, api.BuildOptions{SSHs: tc.cliSSH})
			assert.DeepEqual(t, bake.cfg.Targets["app"].SSH, tc.wantSSH)
			assert.DeepEqual(t, bake.cfg.Targets["app"].Entitlements, []string(nil))
			want := []string{"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", "metadata.json"}
			for _, grant := range tc.wantGrants {
				want = append(want, "--allow", grant)
			}
			assert.DeepEqual(t, bakeArgs(bake, "metadata.json", api.BuildOptions{}), want)
		})
	}
}

func TestLocalCachePaths(t *testing.T) {
	for _, tc := range []struct {
		entry string
		key   string
		want  []string
	}{
		{
			entry: "type=local,src=/cache",
			key:   "src",
			want:  []string{"/cache"},
		},
		{
			entry: "type=local,dest=/cache",
			key:   "dest",
			want:  []string{"/cache"},
		},
		{
			entry: `type=local,"src=/cache,one"`,
			key:   "src",
			want:  []string{"/cache,one"},
		},
		{
			entry: "TYPE=local,SRC=/cache",
			key:   "src",
			want:  []string{"/cache"},
		},
		{
			entry: "type=local,src=/old,src=/new",
			key:   "src",
			want:  []string{"/new"},
		},
		{
			entry: "type=local,src=/cache,type=registry",
			key:   "src",
		},
		{
			entry: "type=local,src=/cache",
			key:   "dest",
		},
		{
			entry: "image:cache",
			key:   "src",
		},
		{
			entry: "type=registry,ref=image:cache",
			key:   "src",
		},
		{
			entry: "type=local,src=/cache,invalid",
			key:   "src",
		},
		{
			entry: `type=local,"src=/cache`,
			key:   "src",
		},
	} {
		t.Run(tc.entry+"/"+tc.key, func(t *testing.T) {
			assert.DeepEqual(t, localCachePaths([]string{tc.entry}, tc.key), tc.want)
		})
	}
}

func TestPrepareBakeCacheGrants(t *testing.T) {
	project := &types.Project{Name: "cache", Services: types.Services{"app": {
		Name: "app", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{
			Context:   "https://example.com/project.git",
			CacheFrom: []string{"type=local,src=/cache/in"}, CacheTo: []string{"type=local,dest=/cache/out"},
		}},
	}}}
	s := &composeService{proxyConfig: map[string]string{}, events: &ignore{}}
	bake := s.prepareBakeBuild(project, project.Services, api.BuildOptions{})
	assert.DeepEqual(t, bakeArgs(bake, "metadata.json", api.BuildOptions{}), []string{
		"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", "metadata.json",
		"--allow", "fs.read=/cache/in", "--allow", "fs.write=/cache/out",
	})
}

func TestLocalBuildContextPaths(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		context string
		want    []string
	}{
		{
			context: "oci-layout:///layouts/image",
			want:    []string{"/layouts/image"},
		},
		{
			context: "oci-layout:///layouts/image:latest",
			want:    []string{"/layouts/image"},
		},
		{
			context: "oci-layout:///layouts/image@" + digest,
			want:    []string{"/layouts/image"},
		},
		{
			context: "oci-layout:///layouts/image:v1@" + digest,
			want:    []string{"/layouts/image"},
		},
		{
			context: "oci-layout:///layouts/@/image",
			want:    []string{"/layouts/@/image"},
		},
		{
			context: `oci-layout://C:\layouts\image`,
			want:    []string{`C:\layouts\image`},
		},
		{
			context: "oci-layout://C:/layouts/image:v1@" + digest,
			want:    []string{"C:/layouts/image"},
		},
		{
			context: "cwd://../local",
			want:    []string{"../local"},
		},
		{
			context: "service:base",
		},
		{
			context: "target:base",
		},
		{
			context: "docker-image:alpine:3.23",
		},
		{
			context: "https://example.com/project.git",
		},
	} {
		t.Run(tc.context, func(t *testing.T) {
			config := types.BuildConfig{Context: "https://example.com/main.git", AdditionalContexts: map[string]string{"base": tc.context}}
			assert.DeepEqual(t, localBuildPaths(&types.Project{}, config), tc.want)
		})
	}
}

func TestBakeTargetNames(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"web":   {},
			"a.b":   {},
			"a_b":   {},
			"a.b.c": {},
			"a_b.c": {},
		},
	}

	names := bakeTargetNames(project)

	// dots are replaced, and services whose names only differ by `.` vs `_`
	// still get distinct bake targets, allocated in sorted service order
	assert.DeepEqual(t, names, map[string]string{
		"web":   "web",
		"a.b":   "a_b",
		"a_b":   "a_b_",
		"a.b.c": "a_b_c",
		"a_b.c": "a_b_c_",
	})
}

func TestToBakeAttest(t *testing.T) {
	tests := []struct {
		name     string
		config   types.BuildConfig
		expected []string
	}{
		{
			name:     "empty — no attest entries",
			config:   types.BuildConfig{},
			expected: nil,
		},
		{
			name:     "provenance true",
			config:   types.BuildConfig{Provenance: "true"},
			expected: []string{"type=provenance"},
		},
		{
			name:     "provenance false — must disable, not omit",
			config:   types.BuildConfig{Provenance: "false"},
			expected: []string{"type=provenance,disabled=true"},
		},
		{
			name:     "provenance mode=max",
			config:   types.BuildConfig{Provenance: "mode=max"},
			expected: []string{"type=provenance,mode=max"},
		},
		{
			name:     "sbom true",
			config:   types.BuildConfig{SBOM: "true"},
			expected: []string{"type=sbom"},
		},
		{
			name:     "sbom false — must disable, not omit",
			config:   types.BuildConfig{SBOM: "false"},
			expected: []string{"type=sbom,disabled=true"},
		},
		{
			name:     "provenance false + sbom false",
			config:   types.BuildConfig{Provenance: "false", SBOM: "false"},
			expected: []string{"type=provenance,disabled=true", "type=sbom,disabled=true"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.DeepEqual(t, toBakeAttest(tc.config), tc.expected)
		})
	}
}

// makeConsole must hand the genuine *os.File over when the stream wraps one:
// on Windows, containerd/console rejects anything but the exact
// os.Stdin/Stdout/Stderr values, so a wrapper would disable the TTY progress
// rendering entirely (#14086).
func TestMakeConsole(t *testing.T) {
	t.Run("stream wrapping a real file yields the file itself", func(t *testing.T) {
		out := makeConsole(streams.NewOut(os.Stdout))
		assert.Equal(t, out, os.Stdout)
	})

	t.Run("file-less stream keeps the console.File wrapper", func(t *testing.T) {
		out := makeConsole(streams.NewOut(&bytes.Buffer{}))
		_, ok := out.(*_console)
		assert.Check(t, ok, "expected a *_console, got %T", out)
	})

	t.Run("plain writer is left untouched", func(t *testing.T) {
		buf := &bytes.Buffer{}
		assert.Equal(t, makeConsole(buf), io.Writer(buf))
	})
}

// A bake that writes a long stream of solve statuses, then why it failed, and
// exits right away, while the consumer of the statuses (the progress display)
// is slower than it: the process is gone when most of the stream is still to
// be read. The error, which comes last, must reach the caller all the same.
func TestWaitBakeKeepsTheEndOfTheStream(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	const statuses = 200
	cmd := exec.CommandContext(t.Context(), "sh", "-c", fmt.Sprintf(`
		i=0; while [ $i -lt %d ]; do echo '{}' >&2; i=$((i+1)); done
		echo 'ERROR: failed to solve: no match for platform in manifest' >&2
		exit 1`, statuses))
	stderr, err := cmd.StderrPipe()
	assert.NilError(t, err)
	assert.NilError(t, cmd.Start())

	ch := make(chan *client.SolveStatus)
	received := make(chan int, 1) // buffered, so the consumer cannot leak if an assertion below aborts the test
	go func() {
		n := 0
		for range ch {
			n++
			time.Sleep(time.Millisecond) // a display slower than bake
		}
		received <- n
	}()

	errMessage, err := waitBake(cmd, stderr, ch)
	close(ch)

	var exitErr *exec.ExitError
	assert.Assert(t, errors.As(err, &exitErr), "the exit status of bake must be reported, got %v", err)
	assert.Equal(t, exitErr.ExitCode(), 1)
	assert.Equal(t, len(errMessage), 1, "the line telling why bake failed was lost: %v", errMessage)
	assert.Assert(t, strings.Contains(errMessage[0], "no match for platform in manifest"), errMessage[0])
	assert.Equal(t, <-received, statuses, "every status must be forwarded")
}
