package execution

type ContainerEvent struct {
	ID     string
	Action string
}

type ContainerExitEvent struct {
	ContainerEvent
	PID        string
	StatusCode uint32
}

const (
	ContainersEventsSubjectSubscriber = "containerd.execution.container.>"
)

const (
	containerEventsSubjectFormat        = "containerd.execution.container.%s"
	containerProcessEventsSubjectFormat = "containerd.execution.container.%s.%s"
)
