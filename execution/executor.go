package execution

import "io"

type CreateOpts struct {
	Bundle string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Executor interface {
	Create(id string, o CreateOpts) (*Container, error)
	List() ([]*Container, error)
	Load(id string) (*Container, error)
	Delete(string) error
}
