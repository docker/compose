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

package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/moby/buildkit/util/gitutil"
)

const GIT_REMOTE_ENABLED = "COMPOSE_EXPERIMENTAL_GIT_REMOTE"

func gitRemoteLoaderEnabled() (bool, error) {
	if v := os.Getenv(GIT_REMOTE_ENABLED); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("COMPOSE_EXPERIMENTAL_GIT_REMOTE environment variable expects boolean value: %w", err)
		}
		return enabled, err
	}
	return false, nil
}

func NewGitRemoteLoader(offline bool) loader.ResourceLoader {
	return gitRemoteLoader{
		offline: offline,
		known:   map[string]string{},
	}
}

type gitRemoteLoader struct {
	offline bool
	known   map[string]string
}

func (g gitRemoteLoader) Accept(path string) bool {
	_, err := gitutil.ParseGitRef(path)
	return err == nil
}

var commitSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)

func (g gitRemoteLoader) Load(ctx context.Context, path string) (string, error) {
	enabled, err := gitRemoteLoaderEnabled()
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("experimental git remote resource is disabled. %q must be set", GIT_REMOTE_ENABLED)
	}

	ref, err := gitutil.ParseGitRef(path)
	if err != nil {
		return "", err
	}

	local, ok := g.known[path]
	if !ok {
		if ref.Commit == "" {
			ref.Commit = "HEAD" // default branch
		}

		err = g.resolveGitRef(ctx, path, ref)
		if err != nil {
			return "", err
		}

		cache, err := cacheDir()
		if err != nil {
			return "", fmt.Errorf("initializing remote resource cache: %w", err)
		}

		local = filepath.Join(cache, ref.Commit)
		if _, err := os.Stat(local); os.IsNotExist(err) {
			if g.offline {
				return "", nil
			}
			err = g.checkout(ctx, local, ref)
			if err != nil {
				return "", err
			}
		}
		g.known[path] = local
	}
	if ref.SubDir != "" {
		local = filepath.Join(local, ref.SubDir)
	}
	stat, err := os.Stat(local)
	if err != nil {
		return "", err
	}
	if stat.IsDir() {
		local, err = findFile(cli.DefaultFileNames, local)
	}
	return local, err
}

func (g gitRemoteLoader) Dir(path string) string {
	return g.known[path]
}

func (g gitRemoteLoader) resolveGitRef(ctx context.Context, path string, ref *gitutil.GitRef) error {
	if !commitSHA.MatchString(ref.Commit) {
		cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", ref.Remote, ref.Commit)
		cmd.Env = g.gitCommandEnv()
		out, err := cmd.Output()
		if err != nil {
			if cmd.ProcessState.ExitCode() == 2 {
				return fmt.Errorf("repository does not contain ref %s, output: %q: %w", path, string(out), err)
			}
			return err
		}
		if len(out) < 40 {
			return fmt.Errorf("unexpected git command output: %q", string(out))
		}
		sha := string(out[:40])
		if !commitSHA.MatchString(sha) {
			return fmt.Errorf("invalid commit sha %q", sha)
		}
		ref.Commit = sha
	}
	return nil
}

func (g gitRemoteLoader) checkout(ctx context.Context, path string, ref *gitutil.GitRef) error {
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		return err
	}
	err = exec.CommandContext(ctx, "git", "init", path).Run()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", "remote", "add", "origin", ref.Remote)
	cmd.Dir = path
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.CommandContext(ctx, "git", "fetch", "--depth=1", "origin", ref.Commit)
	cmd.Env = g.gitCommandEnv()
	cmd.Dir = path
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.CommandContext(ctx, "git", "checkout", ref.Commit)
	cmd.Dir = path
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (g gitRemoteLoader) gitCommandEnv() []string {
	env := types.NewMapping(os.Environ())
	if env["GIT_TERMINAL_PROMPT"] == "" {
		// Disable prompting for passwords by Git until user explicitly asks for it.
		env["GIT_TERMINAL_PROMPT"] = "0"
	}
	if env["GIT_SSH"] == "" && env["GIT_SSH_COMMAND"] == "" {
		// Disable any ssh connection pooling by Git and do not attempt to prompt the user.
		env["GIT_SSH_COMMAND"] = "ssh -o ControlMaster=no -o BatchMode=yes"
	}
	v := env.Values()
	return v
}

func findFile(names []string, pwd string) (string, error) {
	for _, n := range names {
		f := filepath.Join(pwd, n)
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
			return f, nil
		}
	}
	return "", api.ErrNotFound
}

var _ loader.ResourceLoader = gitRemoteLoader{}
