package eventloop

import (
	"runtime"
	"sync"
)

// Event is receiving notification from loop with Handle() call.
type Event interface {
	Handle()
}

// EventLoop is interface for event loops.
// Start starting events processing
// Send adding event to loop
type EventLoop interface {
	Start() error
	Send(Event) error
}

// ChanLoop is implementation of EventLoop based on channels.
type ChanLoop struct {
	events chan Event
	once   sync.Once
}

// NewChanLoop returns ChanLoop with internal channel buffer set to q.
func NewChanLoop(q int) EventLoop {
	return &ChanLoop{
		events: make(chan Event, q),
	}
}

// Start starting to read events from channel in separate goroutines.
// All calls after first is no-op.
func (el *ChanLoop) Start() error {
	go el.once.Do(func() {
		// allocate whole OS thread, so nothing can get scheduled over eventloop
		runtime.LockOSThread()
		for ev := range el.events {
			ev.Handle()
		}
	})
	return nil
}

// Send sends event to channel. Will block if buffer is full.
func (el *ChanLoop) Send(ev Event) error {
	el.events <- ev
	return nil
}
