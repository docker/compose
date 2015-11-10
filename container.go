package containerd

import "os"

type Process interface {
	Pid() int
	Signal(os.Signal) error
}

type Container interface {
	ID() string
	Start() error
	Path() string
	Pid() (int, error)
	SetExited(status int)
	Delete() error
	Processes() ([]Process, error)
}
