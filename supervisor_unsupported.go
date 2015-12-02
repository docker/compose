// +build !libcontainer

package containerd

import (
	"github.com/docker/containerd/runc"
	"github.com/docker/containerd/runtime"
)

func newRuntime(stateDir string) (runtime.Runtime, error) {
	return runc.NewRuntime(stateDir)
}
