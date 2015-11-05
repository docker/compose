package containerd

type Container interface {
	SetExited(status int)
	Delete() error
}
