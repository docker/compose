package oci

import "github.com/docker/containerd/execution"

type containerController struct {
	root string
}

func (c *containerController) Process(container *execution.Container, pid int) (*execution.Process, error) {

}

func (c *containerController) Processes(container *execution.Container) ([]*execution.Process, error) {

}

func (c *containerController) Pause(container *execution.Container) error {
	return command(c.root, "pause", container.ID).Run()
}

func (c *containerController) Resume(container *execution.Container) error {
	return command(c.root, "resume", container.ID).Run()
}
