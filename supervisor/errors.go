package supervisor

import "errors"

var (
	// ErrContainerNotFound is returned when the container ID passed
	// for a given operation is invalid
	ErrContainerNotFound = errors.New("containerd: container not found")
	// ErrProcessNotFound is returned when the process ID passed for
	// a given operation is invalid
	ErrProcessNotFound = errors.New("containerd: process not found for container")
	// ErrUnknownContainerStatus is returned when the container status
	// cannot be determined
	ErrUnknownContainerStatus = errors.New("containerd: unknown container status ")
	// ErrUnknownTask is returned when an unknown Task type is
	// scheduled (should never happen).
	ErrUnknownTask = errors.New("containerd: unknown task type")

	// Internal errors
	errShutdown          = errors.New("containerd: supervisor is shutdown")
	errRootNotAbs        = errors.New("containerd: rootfs path is not an absolute path")
	errNoContainerForPid = errors.New("containerd: pid not registered for any container")
	// internal error where the handler will defer to another for the final response
	//
	// TODO: we could probably do a typed error with another error channel for this to make it
	// less like magic
	errDeferredResponse = errors.New("containerd: deferred response")
)
