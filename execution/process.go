package execution

import (
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type Process interface {
	ID() string
	Pid() int64
	Spec() *specs.Process

	Start() error
	Status() (Status, error)
	Wait() (uint32, error)
	Signal(os.Signal) error
}
