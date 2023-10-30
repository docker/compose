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

package remote

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Based on https://github.com/adrg/xdg
// Licensed under MIT License (MIT)
// Copyright (c) 2014 Adrian-George Bostan <adrg@epistack.com>

func osDependentCacheDir() (string, error) {
	flags := []uint32{windows.KF_FLAG_DEFAULT, windows.KF_FLAG_DEFAULT_PATH}
	for _, flag := range flags {
		p, _ := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, flag|windows.KF_FLAG_DONT_VERIFY)
		if p != "" {
			return filepath.Join(p, "cache", "docker-compose"), nil
		}
	}

	appData, ok := os.LookupEnv("LOCALAPPDATA")
	if ok {
		return filepath.Join(appData, "cache", "docker-compose"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "AppData", "Local", "cache", "docker-compose"), nil
}
