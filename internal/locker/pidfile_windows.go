//go:build windows

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

package locker

import (
	"os"

	"github.com/docker/docker/pkg/pidfile"
	"github.com/mitchellh/go-ps"
)

func (f *Pidfile) Lock() error {
	newPID := os.Getpid()
	err := pidfile.Write(f.path, newPID)
	if err != nil {
		// Get PID registered in the file
		pid, errPid := pidfile.Read(f.path)
		if errPid != nil {
			return err
		}
		// Some users faced issues on Windows where the process written in the pidfile was identified as still existing
		// So we used a 2nd process library to verify if this not a false positive feedback
		// Check if the process exists
		process, errPid := ps.FindProcess(pid)
		if process == nil && errPid == nil {
			// If the process does not exist, remove the pidfile and try to lock again
			_ = os.Remove(f.path)
			return pidfile.Write(f.path, newPID)
		}
	}
	return err
}
