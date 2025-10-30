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
	"testing"

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
