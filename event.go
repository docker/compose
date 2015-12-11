package containerd

import (
	"os"
	"time"

	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/specs"
)

type EventType string

const (
	ExecExitEventType         EventType = "execExit"
	ExitEventType             EventType = "exit"
	StartContainerEventType   EventType = "startContainer"
	DeleteEventType           EventType = "deleteContainerEvent"
	GetContainerEventType     EventType = "getContainer"
	SignalEventType           EventType = "signal"
	AddProcessEventType       EventType = "addProcess"
	UpdateContainerEventType  EventType = "updateContainer"
	CreateCheckpointEventType EventType = "createCheckpoint"
	DeleteCheckpointEventType EventType = "deleteCheckpoint"
)

func NewEvent(t EventType) *Event {
	return &Event{
		Type:      t,
		Timestamp: time.Now(),
		Err:       make(chan error, 1),
	}
}

type Event struct {
	Type       EventType
	Timestamp  time.Time
	ID         string
	BundlePath string
	Pid        int
	Status     int
	Signal     os.Signal
	Process    *specs.Process
	State      *runtime.State
	Containers []runtime.Container
	Checkpoint *runtime.Checkpoint
	Err        chan error
}

type Handler interface {
	Handle(*Event) error
}
