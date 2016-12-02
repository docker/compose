package containerd

import "errors"

var ErrProcessSet = errors.New("container process is already set")

type Executor interface {
	List() ([]*Container, error)
	Load(id string) (*Container, error)

	Create(CreateOpts) (*Container, error)
	Start(string) error
	Delete(string) error
	Exec(string, *Process) (ProcessDelegate, error)
}
