package containerd

import (
	"os"
	"time"

	"github.com/opencontainers/specs"
)

type EventType string

const (
	ExitEventType            EventType = "exit"
	StartContainerEventType  EventType = "startContainer"
	DeleteEventType          EventType = "deleteContainerEvent"
	GetContainerEventType    EventType = "getContainer"
	SignalEventType          EventType = "signal"
	AddProcessEventType      EventType = "addProcess"
	UpdateContainerEventType EventType = "updateContainer"
)

func NewEvent(t EventType) *Event {
	return &Event{
		Type:      t,
		Timestamp: time.Now(),
		Err:       make(chan error, 1),
	}
}

type Stdio struct {
	Stderr string `json:"stderr,omitempty"`
	Stdout string `json:"stdout,omitempty"`
}

type Event struct {
	Type       EventType      `json:"type"`
	Timestamp  time.Time      `json:"timestamp"`
	ID         string         `json:"id,omitempty"`
	BundlePath string         `json:"bundlePath,omitempty"`
	Stdio      *Stdio         `json:"stdio,omitempty"`
	Pid        int            `json:"pid,omitempty"`
	Status     int            `json:"status,omitempty"`
	Signal     os.Signal      `json:"signal,omitempty"`
	Process    *specs.Process `json:"process,omitempty"`
	State      *State         `json:"state,omitempty"`
	Containers []Container    `json:"-"`
	Err        chan error     `json:"-"`
}
