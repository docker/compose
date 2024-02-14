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
	"fmt"
	"os"
	"path/filepath"
)

func greatestExistingAncestor(path string) (string, error) {
	if path == string(filepath.Separator) ||
		path == fmt.Sprintf("%s%s", filepath.VolumeName(path), string(filepath.Separator)) {
		return "", fmt.Errorf("cannot watch root directory")
	}

	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("os.Stat(%q): %w", path, err)
	}

	if os.IsNotExist(err) {
		return greatestExistingAncestor(filepath.Dir(path))
	}

	return path, nil
}
