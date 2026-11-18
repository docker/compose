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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gitutil "github.com/moby/buildkit/frontend/dockerfile/dfgitutil"
	"gotest.tools/v3/assert"
)

// newGitRepository makes a repository of one commit holding a compose file, and
// returns its path and the commit.
func newGitRepository(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	// keep the configuration of the user, which may sign commits, out of the test
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	repo := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte("services:\n  app:\n    image: alpine\n"), 0o600))
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "compose.yaml"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "compose file"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		assert.NilError(t, err, string(out))
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	assert.NilError(t, err)
	return repo, strings.TrimSpace(string(out))
}

func gitCacheNames(t *testing.T, cache string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(cache, cacheGitDir))
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// A checkout is put in the cache directory of Git checkouts, named after the commit,
// only once it is complete: what is left beside it is the entry and nothing else.
func TestGitCheckoutIsPlacedWhenComplete(t *testing.T) {
	repo, sha := newGitRepository(t)
	cache := t.TempDir()

	err := gitRemoteLoader{}.checkout(t.Context(), cache, &gitutil.GitRef{Remote: repo, Ref: sha})
	assert.NilError(t, err)

	assert.DeepEqual(t, gitCacheNames(t, cache), []string{sha})
	_, err = os.Stat(filepath.Join(gitEntryDir(cache, sha), "compose.yaml"))
	assert.NilError(t, err)
}

// A checkout which fails leaves nothing in the cache: no entry for the commit,
// and no directory of the attempt.
func TestGitCheckoutWhichFailsLeavesNothing(t *testing.T) {
	_, sha := newGitRepository(t)
	cache := t.TempDir()

	err := gitRemoteLoader{}.checkout(t.Context(), cache, &gitutil.GitRef{Remote: filepath.Join(cache, "missing"), Ref: sha})
	assert.Assert(t, err != nil)

	assert.Equal(t, len(gitCacheNames(t, cache)), 0)
}

// Runs which check out the same commit together end with one complete entry, and
// none of them fails because another one was first.
func TestGitConcurrentCheckoutsLeaveOneEntry(t *testing.T) {
	repo, sha := newGitRepository(t)
	cache := t.TempDir()

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = gitRemoteLoader{}.checkout(t.Context(), cache, &gitutil.GitRef{Remote: repo, Ref: sha})
		}()
	}
	wg.Wait()

	for _, err := range errs {
		assert.NilError(t, err)
	}
	assert.DeepEqual(t, gitCacheNames(t, cache), []string{sha})
	_, err := os.Stat(filepath.Join(gitEntryDir(cache, sha), "compose.yaml"))
	assert.NilError(t, err)
}

// A directory which an earlier release left at the root of the cache, possibly an
// interrupted checkout, is not taken for the commit: the loader finds nothing to
// load offline, and finds the checkout once it is in the directory for Git.
func TestGitLoaderIgnoresWhatAnEarlierReleaseCached(t *testing.T) {
	repo, sha := newGitRepository(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cache, err := cacheDir()
	assert.NilError(t, err)
	assert.NilError(t, os.MkdirAll(filepath.Join(cache, sha), 0o700))

	remote := "https://example.invalid/repo.git#" + sha
	offline := NewGitRemoteLoader(nil, true)

	local, err := offline.Load(t.Context(), remote)
	assert.NilError(t, err)
	assert.Equal(t, local, "")

	err = gitRemoteLoader{}.checkout(t.Context(), cache, &gitutil.GitRef{Remote: repo, Ref: sha})
	assert.NilError(t, err)

	local, err = NewGitRemoteLoader(nil, true).Load(t.Context(), remote)
	assert.NilError(t, err)
	assert.Equal(t, local, filepath.Join(gitEntryDir(cache, sha), "compose.yaml"))
}
