package watch

import (
	"os"
	"path/filepath"
	"strings"
)

func pathIsChildOf(path string, parent string) bool {
	relPath, err := filepath.Rel(parent, path)
	if err != nil {
		return true
	}

	if relPath == "." {
		return true
	}

	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return false
	}

	return true
}
