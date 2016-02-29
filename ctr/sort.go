package main

import (
	"sort"

	"github.com/docker/containerd/api/grpc/types"
)

func sortContainers(c []*types.Container) {
	sort.Sort(&containerSorter{c})
}

type containerSorter struct {
	c []*types.Container
}

func (s *containerSorter) Len() int {
	return len(s.c)
}

func (s *containerSorter) Swap(i, j int) {
	s.c[i], s.c[j] = s.c[j], s.c[i]
}

func (s *containerSorter) Less(i, j int) bool {
	return s.c[i].Id < s.c[j].Id
}
