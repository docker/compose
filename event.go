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
	StatsEventType            EventType = "events"
	UnsubscribeStatsEventType EventType = "unsubscribeEvents"
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
	Stdout     string
	Stderr     string
	Stdin      string
	Pid        int
	Status     int
	Signal     os.Signal
	Process    *specs.Process
	State      *runtime.State
	Containers []runtime.Container
	Checkpoint *runtime.Checkpoint
	Err        chan error
	Stats      chan interface{}
}

type Handler interface {
	Handle(*Event) error
}
