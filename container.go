package containerd

type Container interface {
	ID() string
	Start() error
	Path() string
	Pid() (int, error)
	SetExited(status int)
	Delete() error
	Processes() ([]int, error)
}
