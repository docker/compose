/*
   Copyright 2024 Docker Compose CLI authors

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

package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/docker/compose/v2/internal/paths"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
)

// fileShareProgressID is the identifier used for the root grouping of file
// share events in the progress writer.
const fileShareProgressID = "Synchronized File Shares"

// RemoveFileSharesForProject removes any Synchronized File Shares that were
// created by Compose for this project in the past if possible.
//
// Errors are not propagated; they are only sent to the progress writer.
func RemoveFileSharesForProject(ctx context.Context, c *Client, projectName string) {
	w := progress.ContextWriter(ctx)

	existing, err := c.ListFileShares(ctx)
	if err != nil {
		w.TailMsgf("Synchronized File Shares not removed due to error: %v", err)
		return
	}
	// filter the list first, so we can early return and not show the event if
	// there's no sessions to clean up
	var toRemove []FileShareSession
	for _, share := range existing {
		if share.Labels["com.docker.compose.project"] == projectName {
			toRemove = append(toRemove, share)
		}
	}
	if len(toRemove) == 0 {
		return
	}

	w.Event(progress.NewEvent(fileShareProgressID, progress.Working, "Removing"))
	rootResult := progress.Done
	defer func() {
		w.Event(progress.NewEvent(fileShareProgressID, rootResult, ""))
	}()
	for _, share := range toRemove {
		shareID := share.Labels["com.docker.desktop.mutagen.file-share.id"]
		if shareID == "" {
			w.Event(progress.Event{
				ID:         share.Alpha.Path,
				ParentID:   fileShareProgressID,
				Status:     progress.Warning,
				StatusText: "Invalid",
			})
			continue
		}

		w.Event(progress.Event{
			ID:       share.Alpha.Path,
			ParentID: fileShareProgressID,
			Status:   progress.Working,
		})

		var status progress.EventStatus
		var statusText string
		if err := c.DeleteFileShare(ctx, shareID); err != nil {
			// TODO(milas): Docker Desktop is doing weird things with error responses,
			// 	once fixed, we can return proper error types from the client
			if strings.Contains(err.Error(), "file share in use") {
				status = progress.Warning
				statusText = "Resource is still in use"
				if rootResult != progress.Error {
					// error takes precedence over warning
					rootResult = progress.Warning
				}
			} else {
				logrus.Debugf("Error deleting file share %q: %v", shareID, err)
				status = progress.Error
				rootResult = progress.Error
			}
		} else {
			logrus.Debugf("Deleted file share: %s", shareID)
			status = progress.Done
		}

		w.Event(progress.Event{
			ID:         share.Alpha.Path,
			ParentID:   fileShareProgressID,
			Status:     status,
			StatusText: statusText,
		})
	}
}

// FileShareManager maps between Compose bind mounts and Desktop File Shares
// state.
type FileShareManager struct {
	mu          sync.Mutex
	cli         *Client
	projectName string
	hostPaths   []string
	// state holds session status keyed by file share ID.
	state map[string]*FileShareSession
}

func NewFileShareManager(cli *Client, projectName string, hostPaths []string) *FileShareManager {
	return &FileShareManager{
		cli:         cli,
		projectName: projectName,
		hostPaths:   hostPaths,
		state:       make(map[string]*FileShareSession),
	}
}

// EnsureExists looks for existing File Shares or creates new ones for the
// host paths.
//
// This function blocks until each share reaches steady state, at which point
// flow can continue.
func (m *FileShareManager) EnsureExists(ctx context.Context) (err error) {
	w := progress.ContextWriter(ctx)
	w.Event(progress.NewEvent(fileShareProgressID, progress.Working, ""))
	defer func() {
		if err != nil {
			w.Event(progress.NewEvent(fileShareProgressID, progress.Error, ""))
		} else {
			w.Event(progress.NewEvent(fileShareProgressID, progress.Done, ""))
		}
	}()

	wait := &waiter{
		shareIDs: make(map[string]struct{}),
		done:     make(chan struct{}),
	}

	handler := m.eventHandler(w, wait)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// stream session events to update internal state for project
	monitorErr := make(chan error, 1)
	go func() {
		defer close(monitorErr)
		if err := m.watch(ctx, handler); err != nil && ctx.Err() == nil {
			monitorErr <- err
		}
	}()

	if err := m.initialize(ctx, wait, handler); err != nil {
		return err
	}

	waitCh := wait.start()
	if waitCh != nil {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case err := <-monitorErr:
			if err != nil {
				return fmt.Errorf("watching file share sessions: %w", err)
			} else if ctx.Err() == nil {
				// this indicates a bug - it should not stop w/o an error if the context is still active
				return errors.New("file share session watch stopped unexpectedly")
			}
		case <-wait.start():
			// everything is done
		}
	}

	return nil
}

// initialize finds existing shares or creates new ones for the host paths.
//
// Once a share is found/created, its progress is monitored via the watch.
func (m *FileShareManager) initialize(ctx context.Context, wait *waiter, handler func(FileShareSession)) error {
	// the watch is already running in the background, so the lock is taken
	// throughout to prevent interleaving writes
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, err := m.cli.ListFileShares(ctx)
	if err != nil {
		return err
	}

	for _, path := range m.hostPaths {
		var fileShareID string
		var fss *FileShareSession

		if fss = findExistingShare(path, existing); fss != nil {
			fileShareID = fss.Beta.Path
			logrus.Debugf("Found existing suitable file share %s for path %q [%s]", fileShareID, path, fss.Alpha.Path)
			wait.addShare(fileShareID)
			handler(*fss)
			continue
		} else {
			req := CreateFileShareRequest{
				HostPath: path,
				Labels: map[string]string{
					"com.docker.compose.project": m.projectName,
				},
			}
			createResp, err := m.cli.CreateFileShare(ctx, req)
			if err != nil {
				return fmt.Errorf("creating file share: %w", err)
			}
			fileShareID = createResp.FileShareID
			fss = m.state[fileShareID]
			logrus.Debugf("Created file share %s for path %q", fileShareID, path)
		}
		wait.addShare(fileShareID)
		if fss != nil {
			handler(*fss)
		}
	}

	return nil
}

func (m *FileShareManager) watch(ctx context.Context, handler func(FileShareSession)) error {
	events, err := m.cli.StreamFileShares(ctx)
	if err != nil {
		return fmt.Errorf("streaming file shares: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-events:
			if event.Error != nil {
				return fmt.Errorf("reading file share events: %w", event.Error)
			}
			// closure for lock
			func() {
				m.mu.Lock()
				defer m.mu.Unlock()
				for _, fss := range event.Value {
					handler(fss)
				}
			}()
		}
	}
}

// eventHandler updates internal state, keeps track of in-progress syncs, and
// prints relevant events to progress.
func (m *FileShareManager) eventHandler(w progress.Writer, wait *waiter) func(fss FileShareSession) {
	return func(fss FileShareSession) {
		fileShareID := fss.Beta.Path

		shouldPrint := wait.isWatching(fileShareID)
		forProject := fss.Labels[api.ProjectLabel] == m.projectName

		if shouldPrint || forProject {
			m.state[fileShareID] = &fss
		}

		var percent int
		var current, total int64
		if fss.Beta.StagingProgress != nil {
			current = int64(fss.Beta.StagingProgress.TotalReceivedSize)
		} else {
			current = int64(fss.Beta.TotalFileSize)
		}
		total = int64(fss.Alpha.TotalFileSize)
		if total != 0 {
			percent = int(current * 100 / total)
		}

		var status progress.EventStatus
		var text string

		switch {
		case strings.HasPrefix(fss.Status, "halted"):
			wait.shareDone(fileShareID)
			status = progress.Error
		case fss.Status == "watching":
			wait.shareDone(fileShareID)
			status = progress.Done
			percent = 100
		case fss.Status == "staging-beta":
			status = progress.Working
			// TODO(milas): the printer doesn't style statuses for children nicely
			text = fmt.Sprintf("    Syncing (%7s / %-7s)",
				units.HumanSize(float64(current)),
				units.HumanSize(float64(total)),
			)
		default:
			// catch-all for various other transitional statuses
			status = progress.Working
		}

		evt := progress.Event{
			ID:       fss.Alpha.Path,
			Status:   status,
			Text:     text,
			ParentID: fileShareProgressID,
			Current:  current,
			Total:    total,
			Percent:  percent,
		}

		if shouldPrint {
			w.Event(evt)
		}
	}
}

func findExistingShare(path string, existing []FileShareSession) *FileShareSession {
	for _, share := range existing {
		if paths.IsChild(share.Alpha.Path, path) {
			return &share
		}
	}
	return nil
}

type waiter struct {
	mu       sync.Mutex
	shareIDs map[string]struct{}
	done     chan struct{}
}

func (w *waiter) addShare(fileShareID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shareIDs[fileShareID] = struct{}{}
}

func (w *waiter) isWatching(fileShareID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.shareIDs[fileShareID]
	return ok
}

// start returns a channel to wait for any outstanding shares to be ready.
//
// If no shares are registered when this is called, nil is returned.
func (w *waiter) start() <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.shareIDs) == 0 {
		return nil
	}
	if w.done == nil {
		w.done = make(chan struct{})
	}
	return w.done
}

func (w *waiter) shareDone(fileShareID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.shareIDs, fileShareID)
	if len(w.shareIDs) == 0 && w.done != nil {
		close(w.done)
		w.done = nil
	}
}
