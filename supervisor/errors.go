package supervisor

import "errors"

var (
	// External errors
	ErrTaskChanNil            = errors.New("containerd: task channel is nil")
	ErrBundleNotFound         = errors.New("containerd: bundle not found")
	ErrContainerNotFound      = errors.New("containerd: container not found")
	ErrContainerExists        = errors.New("containerd: container already exists")
	ErrProcessNotFound        = errors.New("containerd: process not found for container")
	ErrUnknownContainerStatus = errors.New("containerd: unknown container status ")
	ErrUnknownTask            = errors.New("containerd: unknown task type")

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
