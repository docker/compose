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
	"path/filepath"
	"testing"

	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

func TestValidatePathInBase(t *testing.T) {
	base := "/tmp/cache/compose"

	tests := []struct {
		name       string
		unsafePath string
		wantErr    bool
	}{
		{
			name:       "valid simple filename",
			unsafePath: "compose.yaml",
			wantErr:    false,
		},
		{
			name:       "valid hashed filename",
			unsafePath: "f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c.yaml",
			wantErr:    false,
		},
		{
			name:       "valid env file",
			unsafePath: ".env",
			wantErr:    false,
		},
		{
			name:       "valid env file with suffix",
			unsafePath: ".env.prod",
			wantErr:    false,
		},
		{
			name:       "unix path traversal",
			unsafePath: "../../../etc/passwd",
			wantErr:    true,
		},
		{
			name:       "windows path traversal",
			unsafePath: "..\\..\\..\\windows\\system32\\config\\sam",
			wantErr:    true,
		},
		{
			name:       "subdirectory unix",
			unsafePath: "config/base.yaml",
			wantErr:    true,
		},
		{
			name:       "subdirectory windows",
			unsafePath: "config\\base.yaml",
			wantErr:    true,
		},
		{
			name:       "absolute unix path",
			unsafePath: "/etc/passwd",
			wantErr:    true,
		},
		{
			name:       "absolute windows path",
			unsafePath: "C:\\windows\\system32\\config\\sam",
			wantErr:    true,
		},
		{
			name:       "parent reference only",
			unsafePath: "..",
			wantErr:    true,
		},
		{
			name:       "mixed separators",
			unsafePath: "config/sub\\file.yaml",
			wantErr:    true,
		},
		{
			name:       "filename with spaces",
			unsafePath: "my file.yaml",
			wantErr:    false,
		},
		{
			name:       "filename with special chars",
			unsafePath: "file-name_v1.2.3.yaml",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathInBase(base, tt.unsafePath)
			if (err != nil) != tt.wantErr {
				targetPath := filepath.Join(base, tt.unsafePath)
				targetDir := filepath.Dir(targetPath)
				t.Errorf("validatePathInBase(%q, %q) error = %v, wantErr %v\ntargetDir=%q base=%q",
					base, tt.unsafePath, err, tt.wantErr, targetDir, base)
			}
		})
	}
}

func TestWriteComposeFileWithExtendsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a layer with com.docker.compose.extends=true and a path traversal attempt
	layer := spec.Descriptor{
		MediaType: "application/vnd.docker.compose.file.v1+yaml",
		Digest:    "sha256:test123",
		Size:      100,
		Annotations: map[string]string{
			"com.docker.compose.extends": "true",
			"com.docker.compose.file":    "../other",
		},
	}

	content := []byte("services:\n  test:\n    image: nginx\n")

	// writeComposeFile should return an error due to path traversal
	err := writeComposeFile(layer, 0, tmpDir, content)
	assert.Error(t, err, "invalid OCI artifact")
}
