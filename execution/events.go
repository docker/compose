package execution

import "time"

type ContainerEvent struct {
	Timestamp time.Time
	ID        string
	Action    string
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
	containerEventsTopicFormat        = "container.%s"
	containerProcessEventsTopicFormat = "container.%s.%s"
)
