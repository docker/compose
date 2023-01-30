//go:build windows
// +build windows

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
