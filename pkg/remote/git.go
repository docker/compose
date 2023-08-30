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

	"github.com/adrg/xdg"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/moby/buildkit/util/gitutil"
	"github.com/pkg/errors"
)

func GitRemoteLoaderEnabled() (bool, error) {
	if v := os.Getenv("COMPOSE_EXPERIMENTAL_GIT_REMOTE"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return false, errors.Wrap(err, "COMPOSE_EXPERIMENTAL_GIT_REMOTE environment variable expects boolean value")
		}
		return enabled, err
	}
	return false, nil
}

func NewGitRemoteLoader(offline bool) (loader.ResourceLoader, error) {
	// xdg.CacheFile creates the parent directories for the target file path
	// and returns the fully qualified path, so use "git" as a filename and
	// then chop it off after, i.e. no ~/.cache/docker-compose/git file will
	// ever be created
	cache, err := xdg.CacheFile(filepath.Join("docker-compose", "git"))
	if err != nil {
		return nil, fmt.Errorf("initializing git cache: %w", err)
	}
	cache = filepath.Dir(cache)
	return gitRemoteLoader{
		cache:   cache,
		offline: offline,
	}, err
}

type gitRemoteLoader struct {
	cache   string
	offline bool
}

func (g gitRemoteLoader) Accept(path string) bool {
	_, err := gitutil.ParseGitRef(path)
	return err == nil
}

var commitSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)

func (g gitRemoteLoader) Load(ctx context.Context, path string) (string, error) {
	ref, err := gitutil.ParseGitRef(path)
	if err != nil {
		return "", err
	}

	if ref.Commit == "" {
		ref.Commit = "HEAD" // default branch
	}

	if !commitSHA.MatchString(ref.Commit) {
		cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", ref.Remote, ref.Commit)
		cmd.Env = g.gitCommandEnv()
		out, err := cmd.Output()
		if err != nil {
			if cmd.ProcessState.ExitCode() == 2 {
				return "", errors.Wrapf(err, "repository does not contain ref %s, output: %q", path, string(out))
			}
			return "", err
		}
		if len(out) < 40 {
			return "", fmt.Errorf("unexpected git command output: %q", string(out))
		}
		sha := string(out[:40])
		if !commitSHA.MatchString(sha) {
			return "", fmt.Errorf("invalid commit sha %q", sha)
		}
		ref.Commit = sha
	}

	local := filepath.Join(g.cache, ref.Commit)
	if _, err := os.Stat(local); os.IsNotExist(err) {
		if g.offline {
			return "", nil
		}
		err = g.checkout(ctx, local, ref)
		if err != nil {
			return "", err
		}
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
