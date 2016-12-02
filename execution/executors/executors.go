package executors

import "github.com/docker/containerd"

var executors = make(map[string]func() containerd.Executor)

func Register(name string, e func() containerd.Executor) {
	executors[name] = e
}

func Get(name string) func() containerd.Executor {
	return executors[name]
}
