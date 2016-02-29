package supervisor

import (
	"sort"

	"github.com/docker/containerd/runtime"
)

func sortProcesses(p []runtime.Process) {
	sort.Sort(&processSorter{p})
}

type processSorter struct {
	processes []runtime.Process
}

func (s *processSorter) Len() int {
	return len(s.processes)
}

func (s *processSorter) Swap(i, j int) {
	s.processes[i], s.processes[j] = s.processes[j], s.processes[i]
}

func (s *processSorter) Less(i, j int) bool {
	return s.processes[j].ID() == "init"
}
