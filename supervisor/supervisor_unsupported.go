// +build !libcontainer,!runc

package supervisor

import (
	"errors"

	"github.com/docker/containerd/runtime"
)

func newRuntime(stateDir string) (runtime.Runtime, error) {
	return nil, errors.New("unsupported platform")
}
