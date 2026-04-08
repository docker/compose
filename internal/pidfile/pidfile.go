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

// Package pidfile provides helper functions to create and remove PID files.
// A PID file is usually a file used to store the process ID of a running
// process.
//
// This is a temporary copy of github.com/moby/moby/v2/pkg/pidfile, and
// should be replaced once pidfile is available as a standalone module.
package pidfile

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// Read reads the "PID file" at path, and returns the PID if it contains a
// valid PID of a running process, or 0 otherwise. It returns an error when
// failing to read the file, or if the file doesn't exist, but malformed content
// is ignored. Consumers should therefore check if the returned PID is a non-zero
// value before use.
func Read(path string) (pid int, _ error) {
	pidByte, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err = strconv.Atoi(string(bytes.TrimSpace(pidByte)))
	if err != nil {
		return 0, nil
	}
	if pid != 0 && alive(pid) {
		return pid, nil
	}
	return 0, nil
}

// Write writes a "PID file" at the specified path. It returns an error if the
// file exists and contains a valid PID of a running process, or when failing
// to write the file.
func Write(path string, pid int) error {
	if pid < 1 {
		return fmt.Errorf("invalid PID (%d): only positive PIDs are allowed", pid)
	}
	oldPID, err := Read(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if oldPID != 0 {
		return fmt.Errorf("process with PID %d is still running", oldPID)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}
