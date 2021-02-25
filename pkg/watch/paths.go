package watch

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/tilt-dev/tilt/internal/ospath"
)

func greatestExistingAncestor(path string) (string, error) {
	if path == string(filepath.Separator) ||
		path == fmt.Sprintf("%s%s", filepath.VolumeName(path), string(filepath.Separator)) {
		return "", fmt.Errorf("cannot watch root directory")
	}

	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return "", errors.Wrapf(err, "os.Stat(%q)", path)
	}

	if os.IsNotExist(err) {
		return greatestExistingAncestor(filepath.Dir(path))
	}

	return path, nil
}

// If we're recursively watching a path, it doesn't
// make sense to watch any of its descendants.
func dedupePathsForRecursiveWatcher(paths []string) []string {
	result := []string{}
	for _, current := range paths {
		isCovered := false
		hasRemovals := false

		for i, existing := range result {
			if ospath.IsChild(existing, current) {
				// The path is already covered, so there's no need to include it
				isCovered = true
				break
			}

			if ospath.IsChild(current, existing) {
				// Mark the element empty fo removal.
				result[i] = ""
				hasRemovals = true
			}
		}

		if !isCovered {
			result = append(result, current)
		}

		if hasRemovals {
			// Remove all the empties
			newResult := []string{}
			for _, r := range result {
				if r != "" {
					newResult = append(newResult, r)
				}
			}
			result = newResult
		}
	}
	return result
}
