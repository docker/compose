package containerd

import "errors"

var (
	// External errors
	ErrEventChanNil      = errors.New("containerd: event channel is nil")
	ErrBundleNotFound    = errors.New("containerd: bundle not found")
	ErrContainerNotFound = errors.New("containerd: container not found")
	ErrContainerExists   = errors.New("containerd: container already exists")

	// Internal errors
	errShutdown   = errors.New("containerd: supervisor is shutdown")
	errRootNotAbs = errors.New("containerd: rootfs path is not an absolute path")
)
