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

type StartedEvent struct {
	ID        string
	Container Container
}

func (s *StartedEvent) String() string {
	return "started event"
}

type CreateContainerEvent struct {
	ID         string
	BundlePath string
	Err        chan error
}

func (c *CreateContainerEvent) String() string {
	return "create container"
}
