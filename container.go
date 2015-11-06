package containerd

type Container interface {
	ID() string
	Start() error
	Pid() (int, error)
	SetExited(status int)
	Delete() error
}
