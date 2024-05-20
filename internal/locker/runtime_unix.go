//go:build linux || openbsd

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

package locker

import (
	"os"
	"path/filepath"
	"strconv"
)

// Based on https://github.com/adrg/xdg
// Licensed under MIT License (MIT)
// Copyright (c) 2014 Adrian-George Bostan <adrg@epistack.com>

func osDependentRunDir() (string, error) {
	run := filepath.Join("run", "user", strconv.Itoa(os.Getuid()))
	if _, err := os.Stat(run); err == nil {
		return run, nil
	}

	// /run/user/$uid is set by pam_systemd, but might not be present, especially in containerized environments
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".docker", "docker-compose"), nil
}
