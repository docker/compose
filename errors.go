package containerd

import "errors"

var (
	// External errors
	ErrEventChanNil      = errors.New("containerd: event channel is nil")
	ErrBundleNotFound    = errors.New("containerd: bundle not found")
	ErrContainerNotFound = errors.New("containerd: container not found")
	ErrContainerExists   = errors.New("containerd: container already exists")
	ErrProcessNotFound   = errors.New("containerd: processs not found for container")

	// Internal errors
	errShutdown             = errors.New("containerd: supervisor is shutdown")
	errRootNotAbs           = errors.New("containerd: rootfs path is not an absolute path")
	errNoContainerForPid    = errors.New("containerd: pid not registered for any container")
	errInvalidContainerType = errors.New("containerd: invalid container type for runtime")
)
