/*
   Copyright 2018 The Tilt Dev Authors
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/compose-spec/compose-go/v2/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/archive"
)

type archiveEntry struct {
	path   string
	info   os.FileInfo
	header *tar.Header
}

type LowLevelClient interface {
	ContainersForService(ctx context.Context, projectName string, serviceName string) ([]moby.Container, error)

	Exec(ctx context.Context, containerID string, cmd []string, in io.Reader) error
	Untar(ctx context.Context, id string, reader io.ReadCloser) error
}

type Tar struct {
	client LowLevelClient

	projectName string
}

var _ Syncer = &Tar{}

func NewTar(projectName string, client LowLevelClient) *Tar {
	return &Tar{
		projectName: projectName,
		client:      client,
	}
}

func (t *Tar) Sync(ctx context.Context, service types.ServiceConfig, paths []PathMapping) error {
	containers, err := t.client.ContainersForService(ctx, t.projectName, service.Name)
	if err != nil {
		return err
	}

	var pathsToCopy []PathMapping
	var pathsToDelete []string
	for _, p := range paths {
		if _, err := os.Stat(p.HostPath); err != nil && errors.Is(err, fs.ErrNotExist) {
			pathsToDelete = append(pathsToDelete, p.ContainerPath)
		} else {
			pathsToCopy = append(pathsToCopy, p)
		}
	}

	var deleteCmd []string
	if len(pathsToDelete) != 0 {
		deleteCmd = append([]string{"rm", "-rf"}, pathsToDelete...)
	}
	var eg multierror.Group
	for i := range containers {
		containerID := containers[i].ID
		tarReader := tarArchive(pathsToCopy)

		eg.Go(func() error {
			if len(deleteCmd) != 0 {
				if err := t.client.Exec(ctx, containerID, deleteCmd, nil); err != nil {
					return fmt.Errorf("deleting paths in %s: %w", containerID, err)
				}
			}

			if err := t.client.Untar(ctx, containerID, tarReader); err != nil {
				return fmt.Errorf("copying files to %s: %w", containerID, err)
			}
			return nil
		})
	}
	return eg.Wait().ErrorOrNil()
}

type ArchiveBuilder struct {
	tw *tar.Writer
	// A shared I/O buffer to help with file copying.
	copyBuf *bytes.Buffer
}

func NewArchiveBuilder(writer io.Writer) *ArchiveBuilder {
	tw := tar.NewWriter(writer)
	return &ArchiveBuilder{
		tw:      tw,
		copyBuf: &bytes.Buffer{},
	}
}

func (a *ArchiveBuilder) Close() error {
	return a.tw.Close()
}

// ArchivePathsIfExist creates a tar archive of all local files in `paths`. It quietly skips any paths that don't exist.
func (a *ArchiveBuilder) ArchivePathsIfExist(paths []PathMapping) error {
	// In order to handle overlapping syncs, we
	// 1) collect all the entries,
	// 2) de-dupe them, with last-one-wins semantics
	// 3) write all the entries
	//
	// It's not obvious that this is the correct behavior. A better approach
	// (that's more in-line with how syncs work) might ignore files in earlier
	// path mappings when we know they're going to be "synced" over.
	// There's a bunch of subtle product decisions about how overlapping path
	// mappings work that we're not sure about.
	var entries []archiveEntry
	for _, p := range paths {
		newEntries, err := a.entriesForPath(p.HostPath, p.ContainerPath)
		if err != nil {
			return fmt.Errorf("inspecting %q: %w", p.HostPath, err)
		}

		entries = append(entries, newEntries...)
	}

	entries = dedupeEntries(entries)
	for _, entry := range entries {
		err := a.writeEntry(entry)
		if err != nil {
			return fmt.Errorf("archiving %q: %w", entry.path, err)
		}
	}
	return nil
}

func (a *ArchiveBuilder) writeEntry(entry archiveEntry) error {
	pathInTar := entry.path
	header := entry.header

	if header.Typeflag != tar.TypeReg {
		// anything other than a regular file (e.g. dir, symlink) just needs the header
		if err := a.tw.WriteHeader(header); err != nil {
			return fmt.Errorf("writing %q header: %w", pathInTar, err)
		}
		return nil
	}

	file, err := os.Open(pathInTar)
	if err != nil {
		// In case the file has been deleted since we last looked at it.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	defer func() {
		_ = file.Close()
	}()

	// The size header must match the number of contents bytes.
	//
	// There is room for a race condition here if something writes to the file
	// after we've read the file size.
	//
	// For small files, we avoid this by first copying the file into a buffer,
	// and using the size of the buffer to populate the header.
	//
	// For larger files, we don't want to copy the whole thing into a buffer,
	// because that would blow up heap size. There is some danger that this
	// will lead to a spurious error when the tar writer validates the sizes.
	// That error will be disruptive but will be handled as best as we
	// can downstream.
	useBuf := header.Size < 5000000
	if useBuf {
		a.copyBuf.Reset()
		_, err = io.Copy(a.copyBuf, file)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("copying %q: %w", pathInTar, err)
		}
		header.Size = int64(len(a.copyBuf.Bytes()))
	}

	// wait to write the header until _after_ the file is successfully opened
	// to avoid generating an invalid tar entry that has a header but no contents
	// in the case the file has been deleted
	err = a.tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("writing %q header: %w", pathInTar, err)
	}

	if useBuf {
		_, err = io.Copy(a.tw, a.copyBuf)
	} else {
		_, err = io.Copy(a.tw, file)
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("copying %q: %w", pathInTar, err)
	}

	// explicitly flush so that if the entry is invalid we will detect it now and
	// provide a more meaningful error
	if err := a.tw.Flush(); err != nil {
		return fmt.Errorf("finalizing %q: %w", pathInTar, err)
	}
	return nil
}

// tarPath writes the given source path into tarWriter at the given dest (recursively for directories).
// e.g. tarring my_dir --> dest d: d/file_a, d/file_b
// If source path does not exist, quietly skips it and returns no err
func (a *ArchiveBuilder) entriesForPath(localPath, containerPath string) ([]archiveEntry, error) {
	localInfo, err := os.Stat(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	localPathIsDir := localInfo.IsDir()
	if localPathIsDir {
		// Make sure we can trim this off filenames to get valid relative filepaths
		if !strings.HasSuffix(localPath, string(filepath.Separator)) {
			localPath += string(filepath.Separator)
		}
	}

	containerPath = strings.TrimPrefix(containerPath, "/")

	result := make([]archiveEntry, 0)
	err = filepath.Walk(localPath, func(curLocalPath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking %q: %w", curLocalPath, err)
		}

		linkname := ""
		if info.Mode()&os.ModeSymlink != 0 {
			var err error
			linkname, err = os.Readlink(curLocalPath)
			if err != nil {
				return err
			}
		}

		var name string
		//nolint:gocritic
		if localPathIsDir {
			// Name of file in tar should be relative to source directory...
			tmp, err := filepath.Rel(localPath, curLocalPath)
			if err != nil {
				return fmt.Errorf("making %q relative to %q: %w", curLocalPath, localPath, err)
			}
			// ...and live inside `dest`
			name = path.Join(containerPath, filepath.ToSlash(tmp))
		} else if strings.HasSuffix(containerPath, "/") {
			name = containerPath + filepath.Base(curLocalPath)
		} else {
			name = containerPath
		}

		header, err := archive.FileInfoHeader(name, info, linkname)
		if err != nil {
			// Not all types of files are allowed in a tarball. That's OK.
			// Mimic the Docker behavior and just skip the file.
			return nil
		}

		result = append(result, archiveEntry{
			path:   curLocalPath,
			info:   info,
			header: header,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func tarArchive(ops []PathMapping) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		ab := NewArchiveBuilder(pw)
		err := ab.ArchivePathsIfExist(ops)
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("adding files to tar: %w", err))
		} else {
			// propagate errors from the TarWriter::Close() because it performs a final
			// Flush() and any errors mean the tar is invalid
			if err := ab.Close(); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("closing tar: %w", err))
			} else {
				_ = pw.Close()
			}
		}
	}()
	return pr
}

// Dedupe the entries with last-entry-wins semantics.
func dedupeEntries(entries []archiveEntry) []archiveEntry {
	seenIndex := make(map[string]int, len(entries))
	result := make([]archiveEntry, 0, len(entries))
	for i, entry := range entries {
		seenIndex[entry.header.Name] = i
	}
	for i, entry := range entries {
		if seenIndex[entry.header.Name] == i {
			result = append(result, entry)
		}
	}
	return result
}
