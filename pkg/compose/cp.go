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

package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/compose/v2/pkg/progress"
	"golang.org/x/sync/errgroup"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/system"
)

type copyDirection int

const (
	fromService copyDirection = 1 << iota
	toService
	acrossServices = fromService | toService
)

func (s *composeService) Copy(ctx context.Context, projectName string, options api.CopyOptions) error {
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return s.copy(ctx, projectName, options)
	}, s.stdinfo(), "Copying")
}

func (s *composeService) copy(ctx context.Context, projectName string, options api.CopyOptions) error {
	projectName = strings.ToLower(projectName)
	srcService, srcPath := splitCpArg(options.Source)
	destService, dstPath := splitCpArg(options.Destination)

	var direction copyDirection
	var serviceName string
	var copyFunc func(ctx context.Context, containerID string, srcPath string, dstPath string, opts api.CopyOptions) error
	if srcService != "" {
		direction |= fromService
		serviceName = srcService
		copyFunc = s.copyFromContainer

		// copying from multiple containers of a services doesn't make sense.
		if options.All {
			return errors.New("cannot use the --all flag when copying from a service")
		}
	}
	if destService != "" {
		direction |= toService
		serviceName = destService
		copyFunc = s.copyToContainer
	}
	if direction == acrossServices {
		return errors.New("copying between services is not supported")
	}

	if direction == 0 {
		return errors.New("unknown copy direction")
	}

	containers, err := s.listContainersTargetedForCopy(ctx, projectName, options.Index, direction, serviceName)
	if err != nil {
		return err
	}

	w := progress.ContextWriter(ctx)
	g := errgroup.Group{}
	for _, cont := range containers {
		ctr := cont
		g.Go(func() error {
			name := getCanonicalContainerName(ctr)
			var msg string
			if direction == fromService {
				msg = fmt.Sprintf("copy %s:%s to %s", name, srcPath, dstPath)
			} else {
				msg = fmt.Sprintf("copy %s to %s:%s", srcPath, name, dstPath)
			}
			w.Event(progress.Event{
				ID:         name,
				Text:       msg,
				Status:     progress.Working,
				StatusText: "Copying",
			})
			if err := copyFunc(ctx, ctr.ID, srcPath, dstPath, options); err != nil {
				return err
			}
			w.Event(progress.Event{
				ID:         name,
				Text:       msg,
				Status:     progress.Done,
				StatusText: "Copied",
			})
			return nil
		})
	}

	return g.Wait()
}

func (s *composeService) listContainersTargetedForCopy(ctx context.Context, projectName string, index int, direction copyDirection, serviceName string) (Containers, error) {
	var containers Containers
	var err error
	switch {
	case index > 0:
		ctr, err := s.getSpecifiedContainer(ctx, projectName, oneOffExclude, true, serviceName, index)
		if err != nil {
			return nil, err
		}
		return append(containers, ctr), nil
	default:
		containers, err = s.getContainers(ctx, projectName, oneOffExclude, true, serviceName)
		if err != nil {
			return nil, err
		}

		if len(containers) < 1 {
			return nil, fmt.Errorf("no container found for service %q", serviceName)
		}
		if direction == fromService {
			return containers[:1], err

		}
		return containers, err
	}
}

func (s *composeService) copyToContainer(ctx context.Context, containerID string, srcPath string, dstPath string, opts api.CopyOptions) error {
	var err error
	if srcPath != "-" {
		// Get an absolute source path.
		srcPath, err = resolveLocalPath(srcPath)
		if err != nil {
			return err
		}
	}

	// Prepare destination copy info by stat-ing the container path.
	dstInfo := archive.CopyInfo{Path: dstPath}
	dstStat, err := s.apiClient().ContainerStatPath(ctx, containerID, dstPath)

	// If the destination is a symbolic link, we should evaluate it.
	if err == nil && dstStat.Mode&os.ModeSymlink != 0 {
		linkTarget := dstStat.LinkTarget
		if !system.IsAbs(linkTarget) {
			// Join with the parent directory.
			dstParent, _ := archive.SplitPathDirEntry(dstPath)
			linkTarget = filepath.Join(dstParent, linkTarget)
		}

		dstInfo.Path = linkTarget
		dstStat, err = s.apiClient().ContainerStatPath(ctx, containerID, linkTarget)
	}

	// Validate the destination path
	if err := command.ValidateOutputPathFileMode(dstStat.Mode); err != nil {
		return fmt.Errorf(`destination "%s:%s" must be a directory or a regular file: %w`, containerID, dstPath, err)
	}

	// Ignore any error and assume that the parent directory of the destination
	// path exists, in which case the copy may still succeed. If there is any
	// type of conflict (e.g., non-directory overwriting an existing directory
	// or vice versa) the extraction will fail. If the destination simply did
	// not exist, but the parent directory does, the extraction will still
	// succeed.
	if err == nil {
		dstInfo.Exists, dstInfo.IsDir = true, dstStat.Mode.IsDir()
	}

	var (
		content         io.Reader
		resolvedDstPath string
	)

	if srcPath == "-" {
		content = s.stdin()
		resolvedDstPath = dstInfo.Path
		if !dstInfo.IsDir {
			return fmt.Errorf("destination \"%s:%s\" must be a directory", containerID, dstPath)
		}
	} else {
		// Prepare source copy info.
		srcInfo, err := archive.CopyInfoSourcePath(srcPath, opts.FollowLink)
		if err != nil {
			return err
		}

		srcArchive, err := archive.TarResource(srcInfo)
		if err != nil {
			return err
		}
		defer srcArchive.Close() //nolint:errcheck

		// With the stat info about the local source as well as the
		// destination, we have enough information to know whether we need to
		// alter the archive that we upload so that when the server extracts
		// it to the specified directory in the container we get the desired
		// copy behavior.

		// See comments in the implementation of `archive.PrepareArchiveCopy`
		// for exactly what goes into deciding how and whether the source
		// archive needs to be altered for the correct copy behavior when it is
		// extracted. This function also infers from the source and destination
		// info which directory to extract to, which may be the parent of the
		// destination that the user specified.
		// Don't create the archive if running in Dry Run mode
		if !s.dryRun {
			dstDir, preparedArchive, err := archive.PrepareArchiveCopy(srcArchive, srcInfo, dstInfo)
			if err != nil {
				return err
			}
			defer preparedArchive.Close() //nolint:errcheck

			resolvedDstPath = dstDir
			content = preparedArchive
		}
	}

	options := container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
		CopyUIDGID:                opts.CopyUIDGID,
	}
	return s.apiClient().CopyToContainer(ctx, containerID, resolvedDstPath, content, options)
}

func (s *composeService) copyFromContainer(ctx context.Context, containerID, srcPath, dstPath string, opts api.CopyOptions) error {
	var err error
	if dstPath != "-" {
		// Get an absolute destination path.
		dstPath, err = resolveLocalPath(dstPath)
		if err != nil {
			return err
		}
	}

	if err := command.ValidateOutputPath(dstPath); err != nil {
		return err
	}

	// if client requests to follow symbol link, then must decide target file to be copied
	var rebaseName string
	if opts.FollowLink {
		srcStat, err := s.apiClient().ContainerStatPath(ctx, containerID, srcPath)

		// If the destination is a symbolic link, we should follow it.
		if err == nil && srcStat.Mode&os.ModeSymlink != 0 {
			linkTarget := srcStat.LinkTarget
			if !system.IsAbs(linkTarget) {
				// Join with the parent directory.
				srcParent, _ := archive.SplitPathDirEntry(srcPath)
				linkTarget = filepath.Join(srcParent, linkTarget)
			}

			linkTarget, rebaseName = archive.GetRebaseName(srcPath, linkTarget)
			srcPath = linkTarget
		}
	}

	content, stat, err := s.apiClient().CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return err
	}
	defer content.Close() //nolint:errcheck

	if dstPath == "-" {
		_, err = io.Copy(s.stdout(), content)
		return err
	}

	srcInfo := archive.CopyInfo{
		Path:       srcPath,
		Exists:     true,
		IsDir:      stat.Mode.IsDir(),
		RebaseName: rebaseName,
	}

	preArchive := content
	if srcInfo.RebaseName != "" {
		_, srcBase := archive.SplitPathDirEntry(srcInfo.Path)
		preArchive = archive.RebaseArchiveEntries(content, srcBase, srcInfo.RebaseName)
	}

	return archive.CopyTo(preArchive, srcInfo, dstPath)
}

func splitCpArg(arg string) (ctr, path string) {
	if system.IsAbs(arg) {
		// Explicit local absolute path, e.g., `C:\foo` or `/foo`.
		return "", arg
	}

	parts := strings.SplitN(arg, ":", 2)

	if len(parts) == 1 || strings.HasPrefix(parts[0], ".") {
		// Either there's no `:` in the arg
		// OR it's an explicit local relative path like `./file:name.txt`.
		return "", arg
	}

	return parts[0], parts[1]
}

func resolveLocalPath(localPath string) (absPath string, err error) {
	if absPath, err = filepath.Abs(localPath); err != nil {
		return
	}
	return archive.PreserveTrailingDotOrSeparator(absPath, localPath), nil
}
