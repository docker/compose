//go:build windows
// +build windows

package watch

import (
	"github.com/tilt-dev/fsnotify"
)

// TODO(nick): I think the ideal API would be to automatically increase the
// size of the buffer when we exceed capacity. But this gets messy,
// because each time we get a short read error, we need to invalidate
// everything we know about the currently changed files. So for now,
// we just provide a way for people to increase the buffer ourselves.
//
// It might also pay to be clever about sizing the buffer
// relative the number of files in the directory we're watching.
func MaybeIncreaseBufferSize(w *fsnotify.Watcher) {
	w.SetBufferSize(DesiredWindowsBufferSize())
}
