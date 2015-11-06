package containerd

type Container interface {
	ID() string
	Pid() (int, error)
	SetExited(status int)
	Delete() error
}
