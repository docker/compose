package runtime

import "errors"

var (
	ErrNotChildProcess       = errors.New("containerd: not a child process for container")
	ErrInvalidContainerType  = errors.New("containerd: invalid container type for runtime")
	ErrCheckpointNotExists   = errors.New("containerd: checkpoint does not exist for container")
	ErrCheckpointExists      = errors.New("containerd: checkpoint already exists")
	ErrContainerExited       = errors.New("containerd: container has exited")
	ErrTerminalsNotSupported = errors.New("containerd: terminals are not supported for runtime")
	ErrProcessNotExited      = errors.New("containerd: process has not exited")
	ErrProcessExited         = errors.New("containerd: process has exited")

	errNotImplemented = errors.New("containerd: not implemented")
)
