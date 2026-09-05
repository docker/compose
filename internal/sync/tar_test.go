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
	"archive/tar"
	"context"
	"errors"
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
	// untarErrs[i] is what the i-th Untar call returns
	untarErrs []error
	// untarHeaders[i] holds the headers the i-th Untar call received, by entry name
	untarHeaders []map[string]tar.Header
}

func (f *fakeLowLevelClient) ContainersForService(_ context.Context, _ string, _ string) ([]container.Summary, error) {
	return f.containers, nil
}

func (f *fakeLowLevelClient) Exec(_ context.Context, _ string, cmd []string, _ io.Reader) error {
	f.execCmds = append(f.execCmds, cmd)
	return nil
}

func (f *fakeLowLevelClient) Untar(_ context.Context, _ string, reader io.ReadCloser) error {
	headers := map[string]tar.Header{}
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		headers[header.Name] = *header
	}
	f.untarHeaders = append(f.untarHeaders, headers)

	f.untarCount++
	if f.untarCount <= len(f.untarErrs) {
		return f.untarErrs[f.untarCount-1]
	}
	return nil
}

func TestSync_ExistingPath(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	assert.NilError(t, os.WriteFile(existingFile, []byte("data"), 0o644))

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
	}
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
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
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
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
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
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
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: existingFile, ContainerPath: "/app/keep.txt"},
		{HostPath: "/no/such/path", ContainerPath: "/app/removed.txt"},
	})

	assert.NilError(t, err)
	assert.Equal(t, client.untarCount, 1, "existing path should be copied")
	assert.Equal(t, len(client.execCmds), 1)
	assert.Check(t, cmp.Contains(client.execCmds[0][len(client.execCmds[0])-1], "removed.txt"))
}

// A directory that arrives already populated keeps its own header: left implied, the engine
// creates it with ImpliedDirectoryMode and its own root uid/gid, and a non-root service can no
// longer write into what it synced.
func TestSync_PopulatedDirectoryKeepsItsHeader(t *testing.T) {
	tmpDir := populatedSyncDir(t)

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
	}
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: filepath.Join(tmpDir, "sub"), ContainerPath: "/app/sub"},
	})

	assert.NilError(t, err)
	assert.Equal(t, client.untarCount, 1)

	sent := client.untarHeaders[0]
	dir, sentDirHeader := sent["app/sub/"]
	assert.Assert(t, sentDirHeader, "the synced directory needs its own header: %v", sent)
	hostDir, err := os.Stat(filepath.Join(tmpDir, "sub"))
	assert.NilError(t, err)
	assert.Equal(t, dir.FileInfo().Mode().Perm(), hostDir.Mode().Perm(), "the header carries the host mode, not the engine's")
	assert.Equal(t, sent["app/sub/data.txt"].Typeflag, byte(tar.TypeReg))
}

// A directory header for a path the container resolves through a symlink makes the engine reject
// the whole copy (docker/compose#13795). The retry leaves those directories implied, so the files
// land through the symlink.
func TestSync_RetriesWithoutImpliedDirectoriesWhenCopyFails(t *testing.T) {
	tmpDir := populatedSyncDir(t)

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
		untarErrs:  []error{errors.New(`cannot overwrite non-directory "/app/sub" with directory "/"`)},
	}
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: filepath.Join(tmpDir, "sub"), ContainerPath: "/app/sub"},
	})

	assert.NilError(t, err)
	assert.Equal(t, client.untarCount, 2)

	retried := client.untarHeaders[1]
	_, sentDirHeader := retried["app/sub/"]
	assert.Check(t, !sentDirHeader, "no header expected for the directory holding the synced files: %v", retried)
	assert.Equal(t, retried["app/sub/data.txt"].Typeflag, byte(tar.TypeReg))
	assert.Equal(t, retried["app/sub/nested/"].Typeflag, byte(tar.TypeDir), "an empty directory still needs its own header")
}

func TestSync_ReportsTheCopyErrorWhenTheRetryFailsToo(t *testing.T) {
	tmpDir := populatedSyncDir(t)

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
		untarErrs: []error{
			errors.New(`cannot overwrite non-directory "/app/sub" with directory "/"`),
			errors.New("no such container"),
		},
	}
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: filepath.Join(tmpDir, "sub"), ContainerPath: "/app/sub"},
	})

	assert.ErrorContains(t, err, "cannot overwrite non-directory")
	assert.ErrorContains(t, err, "no such container")
	assert.Equal(t, client.untarCount, 2)
}

// Retrying drops the directory headers, so only the failure they cause may be retried.
func TestSync_DoesNotRetryACopyThatFailedForAnotherReason(t *testing.T) {
	tmpDir := populatedSyncDir(t)

	client := &fakeLowLevelClient{
		containers: []container.Summary{{ID: "ctr1"}},
		untarErrs:  []error{errors.New("no such container")},
	}
	syncer := NewTar("proj", client)

	err := syncer.Sync(t.Context(), "svc", []*PathMapping{
		{HostPath: filepath.Join(tmpDir, "sub"), ContainerPath: "/app/sub"},
	})

	assert.ErrorContains(t, err, "no such container")
	assert.Equal(t, client.untarCount, 1)
	assert.Equal(t, client.untarHeaders[0]["app/sub/"].Typeflag, byte(tar.TypeDir), "the archive sent must be the unreduced one")
}

// sub/ holds a file and an empty directory, the shapes the archive treats differently.
func populatedSyncDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	assert.NilError(t, os.MkdirAll(filepath.Join(tmpDir, "sub", "nested"), 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(tmpDir, "sub", "data.txt"), []byte("data"), 0o644))
	return tmpDir
}
