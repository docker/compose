package supervisor

import "github.com/cloudfoundry/gosigar"

type Machine struct {
	Cpus   int
	Memory int64
}

func CollectMachineInformation() (Machine, error) {
	m := Machine{}
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
