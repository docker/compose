//go:build !windows

/*
   Copyright 2025 Docker Compose CLI authors

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

package pidfile

import (
	"errors"
	"os"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

func alive(pid int) bool {
	if pid < 1 {
		return false
	}
	switch runtime.GOOS {
	case "darwin":
		err := unix.Kill(pid, 0)
		return err == nil || errors.Is(err, unix.EPERM)
	default:
		_, err := os.Stat("/proc/" + strconv.Itoa(pid))
		return err == nil
	}
}
