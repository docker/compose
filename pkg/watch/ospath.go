package watch

import "github.com/windmilleng/tilt/internal/ospath"

func pathIsChildOf(path string, parent string) bool {
	_, isChild := ospath.Child(parent, path)
	return isChild
}
