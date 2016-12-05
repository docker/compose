package executors

import "github.com/docker/containerd/execution"

var executors = make(map[string]func() execution.Executor)

func Register(name string, e func() execution.Executor) {
	executors[name] = e
}

func Get(name string) func() execution.Executor {
	return executors[name]
}
