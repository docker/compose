// +build libcontainer

package supervisor

import (
	"github.com/docker/containerd/linux"
	"github.com/docker/containerd/runtime"
)

func newRuntime(stateDir string) (runtime.Runtime, error) {
	return linux.NewRuntime(stateDir)
}
