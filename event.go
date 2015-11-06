package containerd

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
