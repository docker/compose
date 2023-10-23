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
)

func cacheDir() (string, error) {
	cache, ok := os.LookupEnv("XDG_CACHE_HOME")
	if ok {
		return filepath.Join(cache, "docker-compose"), nil
	}

	path, err := osDependentCacheDir()
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(path, 0o700)
	return path, err
}
