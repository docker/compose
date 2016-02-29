package supervisor

import "github.com/cloudfoundry/gosigar"

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
	m.Memory = int64(mem.Total / 1024 / 1024)
	return m, nil
}
