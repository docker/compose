package supervisor

import (
	"errors"
)

type Machine struct {
	Cpus   int
	Memory int64
}

func CollectMachineInformation() (Machine, error) {
	m := Machine{}
	return m, errors.New("supervisor CollectMachineInformation not implemented on Solaris")
}
