/*
   Copyright 2023 Docker Compose CLI authors

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

package sync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// fakeLowLevelClient records calls made to it for test assertions.
type fakeLowLevelClient struct {
	containers []container.Summary
	execCmds   [][]string
	untarCount int
}

func (f *fakeLowLevelClient) ContainersForService(_ context.Context, _ string, _ string) ([]container.Summary, error) {
	return f.containers, nil
}

func (f *fakeLowLevelClient) Exec(_ context.Context, _ string, cmd []string, _ io.Reader) error {
	f.execCmds = append(f.execCmds, cmd)
	return nil
}

func (f *fakeLowLevelClient) Untar(_ context.Context, _ string, _ io.ReadCloser) error {
	f.untarCount++
	return nil
}

func TestSync_ExistingPath(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	assert.NilError(t, os.WriteFile(existingFile, []byte("data"), 0o644))

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
	}
	tar := NewTar("proj", client)

	err := tar.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: existingFile, ContainerPath: "/app/exists.txt"},
	})

	assert.NilError(t, err)
	assert.Equal(t, client.untarCount, 1, "existing path should be copied via Untar")
	assert.Equal(t, len(client.execCmds), 0, "no delete command expected for existing path")
}

func TestSync_NonExistentPath(t *testing.T) {
	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
	}
	tar := NewTar("proj", client)

	err := tar.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: "/no/such/file", ContainerPath: "/app/gone.txt"},
	})

	assert.NilError(t, err)
	assert.Equal(t, len(client.execCmds), 1, "should issue a delete command")
	assert.DeepEqual(t, client.execCmds[0], []string{"rm", "-rf", "/app/gone.txt"})
}

func TestSync_StatPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("test requires non-root to trigger EACCES")
	}

	tmpDir := t.TempDir()
	restrictedDir := filepath.Join(tmpDir, "noaccess")
	assert.NilError(t, os.Mkdir(restrictedDir, 0o700))
	targetFile := filepath.Join(restrictedDir, "secret.txt")
	assert.NilError(t, os.WriteFile(targetFile, []byte("data"), 0o644))
	// Remove all permissions on the parent directory so stat on the child fails with EACCES.
	assert.NilError(t, os.Chmod(restrictedDir, 0o000))
	t.Cleanup(func() {
		// Restore permissions so t.TempDir() cleanup can remove it.
		_ = os.Chmod(restrictedDir, 0o700)
	})

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
	}
	tar := NewTar("proj", client)

	err := tar.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: targetFile, ContainerPath: "/app/secret.txt"},
	})

	assert.ErrorContains(t, err, "permission denied")
	assert.ErrorContains(t, err, "secret.txt")
	assert.Equal(t, client.untarCount, 0, "should not attempt copy on stat error")
	assert.Equal(t, len(client.execCmds), 0, "should not attempt delete on stat error")
}

func TestSync_MixedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "keep.txt")
	assert.NilError(t, os.WriteFile(existingFile, []byte("data"), 0o644))

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
	}
	tar := NewTar("proj", client)

	err := tar.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: existingFile, ContainerPath: "/app/keep.txt"},
		{HostPath: "/no/such/path", ContainerPath: "/app/removed.txt"},
	})

	assert.NilError(t, err)
	assert.Equal(t, client.untarCount, 1, "existing path should be copied")
	assert.Equal(t, len(client.execCmds), 1)
	assert.Check(t, cmp.Contains(client.execCmds[0][len(client.execCmds[0])-1], "removed.txt"))
}
