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
	"os/exec"
	"strings"
	"testing"

	gitutil "github.com/moby/buildkit/frontend/dockerfile/dfgitutil"
	"gotest.tools/v3/assert"
)

func TestValidateGitSubDir(t *testing.T) {
	base := "/tmp/cache/compose/abc123def456"

	tests := []struct {
		name    string
		subDir  string
		wantErr bool
	}{
		{
			name:    "valid simple directory",
			subDir:  "examples",
			wantErr: false,
		},
		{
			name:    "valid nested directory",
			subDir:  "examples/nginx",
			wantErr: false,
		},
		{
			name:    "valid deeply nested directory",
			subDir:  "examples/web/frontend/config",
			wantErr: false,
		},
		{
			name:    "valid current directory",
			subDir:  ".",
			wantErr: false,
		},
		{
			name:    "valid directory with redundant separators",
			subDir:  "examples//nginx",
			wantErr: false,
		},
		{
			name:    "valid directory with dots in name",
			subDir:  "examples/nginx.conf.d",
			wantErr: false,
		},
		{
			name:    "path traversal - parent directory",
			subDir:  "..",
			wantErr: true,
		},
		{
			name:    "path traversal - multiple parent directories",
			subDir:  "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal - deeply nested escape",
			subDir:  "../../../../../../../tmp/pwned",
			wantErr: true,
		},
		{
			name:    "path traversal - mixed with valid path",
			subDir:  "examples/../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal - at the end",
			subDir:  "examples/..",
			wantErr: false, // This resolves to "." which is the current directory, safe
		},
		{
			name:    "path traversal - in the middle",
			subDir:  "examples/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal - windows style",
			subDir:  "..\\..\\..\\windows\\system32",
			wantErr: true,
		},
		{
			name:    "absolute unix path",
			subDir:  "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute windows path",
			subDir:  "C:\\windows\\system32\\config\\sam",
			wantErr: true,
		},
		{
			name:    "absolute path with home directory",
			subDir:  "/home/user/.ssh/id_rsa",
			wantErr: true,
		},
		{
			name:    "normalized path that would escape",
			subDir:  "./../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "directory name with three dots",
			subDir:  ".../config",
			wantErr: false,
		},
		{
			name:    "directory name with four dots",
			subDir:  "..../config",
			wantErr: false,
		},
		{
			name:    "directory name with five dots",
			subDir:  "...../etc/passwd",
			wantErr: false, // ".....'' is a valid directory name, not path traversal
		},
		{
			name:    "directory name starting with two dots and letter",
			subDir:  "..foo/bar",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitSubDir(base, tt.subDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGitSubDir(%q, %q) error = %v, wantErr %v",
					base, tt.subDir, err, tt.wantErr)
			}
		})
	}
}

// TestValidateGitSubDirSecurityScenarios tests specific security scenarios
func TestValidateGitSubDirSecurityScenarios(t *testing.T) {
	base := "/var/cache/docker-compose/git/1234567890abcdef"

	// Test the exact vulnerability scenario from the issue
	t.Run("CVE scenario - /tmp traversal", func(t *testing.T) {
		maliciousPath := "../../../../../../../tmp/pwned"
		err := validateGitSubDir(base, maliciousPath)
		assert.ErrorContains(t, err, "path traversal")
	})

	// Test variations of the attack
	t.Run("CVE scenario - /etc traversal", func(t *testing.T) {
		maliciousPath := "../../../../../../../../etc/passwd"
		err := validateGitSubDir(base, maliciousPath)
		assert.ErrorContains(t, err, "path traversal")
	})

	// Test that legitimate nested paths still work
	t.Run("legitimate nested path", func(t *testing.T) {
		validPath := "examples/docker-compose/nginx/config"
		err := validateGitSubDir(base, validPath)
		assert.NilError(t, err)
	})
}

func TestResolveGitRefPicksExactRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com"}, args...)...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		assert.NilError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "on main")
	// ls-remote matches "main" against the tail of every ref, and
	// refs/heads/feature/main sorts before refs/heads/main.
	git("checkout", "-q", "-b", "feature/main")
	git("commit", "-q", "--allow-empty", "-m", "on feature/main")
	git("tag", "v1")
	git("checkout", "-q", "main")
	git("commit", "-q", "--allow-empty", "-m", "on release/v1")
	git("branch", "release/v1")
	mainSHA := git("rev-parse", "main")
	tagSHA := git("rev-parse", "v1")

	tests := []struct {
		ref  string
		want string
	}{
		{ref: "main", want: mainSHA},
		{ref: "refs/heads/main", want: mainSHA},
		{ref: "v1", want: tagSHA},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			ref := &gitutil.GitRef{Remote: repo, Ref: tt.ref}
			err := gitRemoteLoader{}.resolveGitRef(t.Context(), repo+"#"+tt.ref, ref)
			assert.NilError(t, err)
			assert.Equal(t, ref.Ref, tt.want)
		})
	}
}

func TestMatchLsRemoteRef(t *testing.T) {
	out := "1111111111111111111111111111111111111111\trefs/heads/feature/main\n" +
		"2222222222222222222222222222222222222222\trefs/heads/main\n" +
		"3333333333333333333333333333333333333333\trefs/tags/main\n"
	sha, err := matchLsRemoteRef(out, "main")
	assert.NilError(t, err)
	assert.Equal(t, sha, "3333333333333333333333333333333333333333", "git prefers a tag over a branch with the same name")

	sha, err = matchLsRemoteRef(out, "refs/heads/main")
	assert.NilError(t, err)
	assert.Equal(t, sha, "2222222222222222222222222222222222222222")

	_, err = matchLsRemoteRef(out, "feature")
	assert.ErrorContains(t, err, "unexpected git command output")
}
