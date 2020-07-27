// +build !windows

package watch

import "github.com/tilt-dev/fsnotify"

func MaybeIncreaseBufferSize(w *fsnotify.Watcher) {
	// Not needed on non-windows
}
