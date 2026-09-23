//go:build e2e

/*
Copyright 2026 Docker Compose CLI authors

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
package e2e

import (
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitRepo is a throwaway git repository served over the smart HTTP protocol
// by an in-process server: `git http-backend` run as a CGI per request, on a
// random port picked by httptest. No daemon process, no container — hermetic
// and port-collision free. The smart protocol is required: compose's git
// loader resolves the ref with ls-remote then shallow-fetches the raw commit,
// which the dumb protocol supports neither of (no shallow capability), and
// fetching a commit by hash needs uploadpack.allowAnySHA1InWant.
type gitRepo struct {
	t    *testing.T
	work string // working tree the fixture content is committed from
	bare string // bare repository the server exposes
	URL  string // smart-HTTP URL of the repository (…/repo.git)
}

// serveGitRepo commits the content of dir on a `main` branch and serves the
// resulting repository over smart HTTP for the lifetime of the test.
func serveGitRepo(t *testing.T, dir string) *gitRepo {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available in PATH")
	}
	root := t.TempDir()
	r := &gitRepo{t: t, work: dir, bare: filepath.Join(root, "repo.git")}
	// init then set HEAD explicitly: `git init -b` requires git >= 2.28,
	// symbolic-ref names the initial branch on any version
	r.git(dir, "init", "-q", ".")
	r.git(dir, "symbolic-ref", "HEAD", "refs/heads/main")
	r.git(dir, "add", "-A")
	r.git(dir, "commit", "-q", "-m", "e2e fixture")
	r.git(dir, "clone", "-q", "--bare", ".", r.bare)
	r.git(r.bare, "config", "uploadpack.allowAnySHA1InWant", "true")

	server := httptest.NewServer(&cgi.Handler{
		Path: gitPath,
		Args: []string{"http-backend"},
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"},
	})
	t.Cleanup(server.Close)
	r.URL = server.URL + "/repo.git"
	return r
}

// Branch publishes a variant of the fixture under a new branch: mutate edits
// the working tree, and the resulting commit is pushed to the served
// repository.
func (r *gitRepo) Branch(name string, mutate func(dir string)) {
	r.t.Helper()
	r.git(r.work, "checkout", "-q", "-b", name)
	mutate(r.work)
	r.git(r.work, "add", "-A")
	r.git(r.work, "commit", "-q", "-m", "branch "+name)
	r.git(r.work, "push", "-q", r.bare, name)
	// return to main so each Branch call cuts from the same base
	r.git(r.work, "checkout", "-q", "main")
}

// git runs a git command against a fully isolated configuration: no user or
// system gitconfig (so a developer's signing or hook setup cannot leak into
// the fixture) and a fixed identity.
func (r *gitRepo) git(dir string, args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=compose-e2e",
		"GIT_AUTHOR_EMAIL=e2e@compose.invalid",
		"GIT_COMMITTER_NAME=compose-e2e",
		"GIT_COMMITTER_EMAIL=e2e@compose.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestGitRemoteUp(t *testing.T) {
	s := NewScenario(t, "up on a git remote must deploy the project, resolving its files against the fetched copy")
	repo := serveGitRepo(t, s.Dir())
	s.Env("XDG_CACHE_HOME=" + t.TempDir())
	s.FromRemote(repo.URL)
	s.Step("up fetches the repository and starts the service",
		ComposeCmd("up", "-d", "--wait", "--yes").Within(60*time.Second),
		ServiceState("app", "running"),
		// the env_file exists only inside the repository: its effect proves
		// the relative reference was resolved against the fetched copy
		ContainerEnv("app", "FLAVOR", "main"))
}

func TestGitRemoteBranchSelection(t *testing.T) {
	s := NewScenario(t, "a #branch fragment on a git remote must deploy that branch's revision of the project")
	repo := serveGitRepo(t, s.Dir())
	repo.Branch("feature", func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "app.env"), []byte("FLAVOR=feature\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	s.Env("XDG_CACHE_HOME=" + t.TempDir())
	s.FromRemote(repo.URL + "#feature")
	s.Step("up deploys the feature branch, not the default one",
		ComposeCmd("up", "-d", "--wait", "--yes").Within(60*time.Second),
		ServiceState("app", "running"),
		ContainerEnv("app", "FLAVOR", "feature"))
}

func TestGitRemoteSubdir(t *testing.T) {
	s := NewScenario(t, "a #ref:subdir fragment must load the project from the repository subdirectory, not its root")
	repo := serveGitRepo(t, s.Dir())
	s.Env("XDG_CACHE_HOME=" + t.TempDir())
	s.FromRemote(repo.URL + "#main:apps/web")
	s.Step("up deploys the subdirectory project, ignoring the decoy at the repository root",
		ComposeCmd("up", "-d", "--wait", "--yes").Within(60*time.Second),
		ServiceState("app", "running"),
		ContainerEnv("app", "FLAVOR", "web"))
}
