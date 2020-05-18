package containers

import (
	"context"
	"io"
)

// Container represents a created container
type Container struct {
	ID          string
	Status      string
	Image       string
	Command     string
	CPUTime     uint64
	MemoryUsage uint64
	MemoryLimit uint64
	PidsCurrent uint64
	PidsLimit   uint64
	Labels      []string
	Ports       []Port
}

// Port represents a published port of a container
type Port struct {
	// HostPort is the port number on the host
	HostPort uint32
	// ContainerPort is the port number inside the container
	ContainerPort uint32
	/// Protocol is the protocol of the port mapping
	Protocol string
	// HostIP is the host ip to use
	HostIP string
}

// ContainerConfig contains the configuration data about a container
type ContainerConfig struct {
	// ID uniquely identifies the container
	ID string
	// Image specifies the iamge reference used for a container
	Image string
	// Ports provide a list of published ports
	Ports []Port
	// Labels set labels to the container
	Labels map[string]string
}

// LogsRequest contains configuration about a log request
type LogsRequest struct {
	Follow bool
	Tail   string
	Writer io.Writer
}

// Service interacts with the underlying container backend
type Service interface {
	// List returns all the containers
	List(ctx context.Context, all bool) ([]Container, error)
	// Stop stops the running container
	Stop(ctx context.Context, containerID string, timeout *uint32) error
	// Run creates and starts a container
	Run(ctx context.Context, config ContainerConfig) error
	// Exec executes a command inside a running container
	Exec(ctx context.Context, containerName string, command string, reader io.Reader, writer io.Writer) error
	// Logs returns all the logs of a container
	Logs(ctx context.Context, containerName string, request LogsRequest) error
	// Delete removes containers
	Delete(ctx context.Context, id string, force bool) error
}
