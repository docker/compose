//go:build windows

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

import "golang.org/x/sys/windows"

func alive(pid int) bool {
	if pid < 1 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	var c uint32
	err = windows.GetExitCodeProcess(h, &c)
	_ = windows.CloseHandle(h)
	if err != nil {
		return c == uint32(windows.STATUS_PENDING)
	}
	return true
}
