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

type Port struct {
	Source      uint32
	Destination uint32
}

type ContainerConfig struct {
	ID    string
	Image string
	Ports []Port
}

type ContainerService interface {
	List(context.Context) ([]Container, error)
	Run(context.Context, ContainerConfig) error
}
