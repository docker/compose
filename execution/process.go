package execution

import "os"

type Process interface {
	ID() string
	Pid() int64
	//Spec() *specs.Process
	Wait() (uint32, error)
	Signal(os.Signal) error
	Status() Status
}
