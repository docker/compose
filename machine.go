package containerd

import "github.com/cloudfoundry/gosigar"

type Machine struct {
	ID     string
	Cpus   int
	Memory int64
}

func CollectMachineInformation(id string) (Machine, error) {
	m := Machine{
		ID: id,
	}
	cpu := sigar.CpuList{}
	if err := cpu.Get(); err != nil {
		return m, err
	}
	m.Cpus = len(cpu.List)
	mem := sigar.Mem{}
	if err := mem.Get(); err != nil {
		return m, err
	}
	m.Memory = int64(mem.Total)
	return m, nil
}
