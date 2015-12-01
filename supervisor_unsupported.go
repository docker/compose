// +build !libcontainer

package containerd

import (
	"errors"

	"github.com/docker/containerd/runtime"
)

func newRuntime(stateDir string) (runtime.Runtime, error) {
	return nil, errors.New("Unsupported runtime")
}
