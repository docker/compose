package containerd

type Process interface {
	// Signal sends a signal to the process.
	SetExited(status int)
}
