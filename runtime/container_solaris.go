package runtime

import (
	"errors"

	"github.com/docker/containerd/specs"
)

func (c *container) OOM() (OOM, error) {
	return nil, errors.New("runtime OOM() not implemented on Solaris")
}

func (c *container) UpdateResources(r *Resource) error {
	return errors.New("runtime UpdateResources() not implemented on Solaris")
}

func getRootIDs(s *specs.Spec) (int, int, error) {
	return 0, 0, errors.New("runtime getRootIDs() not implemented on Solaris")
}
