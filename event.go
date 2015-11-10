package containerd

import "os"

type Event interface {
	String() string
}

type CallbackEvent interface {
	Event() Event
	Callback() chan Event
}

type ExitEvent struct {
	Pid    int
	Status int
}

func (e *ExitEvent) String() string {
	return "exit event"
}

type StartContainerEvent struct {
	ID         string
	BundlePath string
	Err        chan error
}

func (c *StartContainerEvent) String() string {
	return "create container"
}

type ContainerStartErrorEvent struct {
	ID string
}

func (c *ContainerStartErrorEvent) String() string {
	return "container start error"
}

type GetContainersEvent struct {
	Containers []Container
	Err        chan error
}

func (c *GetContainersEvent) String() string {
	return "get containers"
}

type SignalEvent struct {
	ID     string
	Pid    int
	Signal os.Signal
	Err    chan error
}

func (s *SignalEvent) String() string {
	return "signal event"
}
