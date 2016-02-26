package supervisor

import (
	"errors"

	"github.com/docker/containerd/runtime"
)

// TODO Windows: This is going to be very problematic to port to Windows.
// Windows golang has no concept of EpollEvent/EpollCtl etc as in the
// Linux implementation. @crosbymichael - Help needed.

func NewMonitor() (*Monitor, error) {
	return nil, errors.New("NewMonitor not implemented on Windows")
}

type Monitor struct {
}

func (m *Monitor) Exits() chan runtime.Process {
	return nil
}

func (m *Monitor) Monitor(p runtime.Process) error {
	return errors.New("Monitor not implemented on Windows")
}

func (m *Monitor) Close() error {
	return errors.New("Monitor Close() not implemented on Windows")
}

func (m *Monitor) start() {
}
