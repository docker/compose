package containers

import (
	"context"
)

type Container struct {
	ID          string
	Status      string
	Image       string
	Command     string
	CpuTime     uint64
	MemoryUsage uint64
	MemoryLimit uint64
	PidsCurrent uint64
	PidsLimit   uint64
	Labels      []string
}

type ContainerService interface {
	List(context.Context) ([]Container, error)
}
