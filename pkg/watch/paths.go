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

	pathutil "github.com/docker/compose/v5/internal/paths"
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

func greatestExistingAncestors(paths []string, ignoreList map[string]PathMatcher) ([]string, error) {
	result := []string{}
	for _, path := range paths {
		newP, err := greatestExistingAncestor(path)
		if err != nil {
			return nil, fmt.Errorf("finding ancestor of %s: %w", path, err)
		}
		result = append(result, newP)
		if path != newP {
			ignore := ignoreList[path]
			if oldMatcher, exists := ignoreList[newP]; exists {
				ignore = NewIntersectMatcher(oldMatcher, ignore)
			}
			ignoreList[newP] = ignore
			delete(ignoreList, path)
		}
	}
	return result, nil
}

func normalizeWatchRoots(paths []string, ignore map[string]PathMatcher) (map[string]bool, map[string]PathMatcher, error) {
	notifyList := make(map[string]bool, len(paths))
	normalizedIgnores := make(map[string]PathMatcher, len(paths))

	for _, root := range paths {
		root, err := filepath.Abs(root)
		if err != nil {
			return nil, nil, err
		}
		notifyList[root] = true

		matchers := make([]PathMatcher, 0, len(ignore))
		for triggerPath, matcher := range ignore {
			if matcher == nil {
				continue
			}
			if root == triggerPath || pathutil.IsChild(root, triggerPath) || pathutil.IsChild(triggerPath, root) {
				matchers = append(matchers, matcher)
			}
		}
		normalizedIgnores[root] = NewIntersectMatcher(matchers...)
	}
	return notifyList, normalizedIgnores, nil
}
