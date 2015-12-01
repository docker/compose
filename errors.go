package containerd

import "errors"

var (
	// External errors
	ErrEventChanNil           = errors.New("containerd: event channel is nil")
	ErrBundleNotFound         = errors.New("containerd: bundle not found")
	ErrContainerNotFound      = errors.New("containerd: container not found")
	ErrContainerExists        = errors.New("containerd: container already exists")
	ErrProcessNotFound        = errors.New("containerd: processs not found for container")
	ErrUnknownContainerStatus = errors.New("containerd: unknown container status ")
	ErrUnknownEvent           = errors.New("containerd: unknown event type")

	// Internal errors
	errShutdown             = errors.New("containerd: supervisor is shutdown")
	errRootNotAbs           = errors.New("containerd: rootfs path is not an absolute path")
	errNoContainerForPid    = errors.New("containerd: pid not registered for any container")
	errInvalidContainerType = errors.New("containerd: invalid container type for runtime")
	errNotChildProcess      = errors.New("containerd: not a child process for container")
	// internal error where the handler will defer to another for the final response
	//
	// TODO: we could probably do a typed error with another error channel for this to make it
	// less like magic
	errDeferedResponse = errors.New("containerd: defered response")
)
