package supervisor

import (
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/archutils"
	"github.com/docker/containerd/runtime"
)

// NewMonitor starts a new process monitor and returns it
func NewMonitor() (*Monitor, error) {
	m := &Monitor{
		receivers: make(map[int]interface{}),
		exits:     make(chan runtime.Process, 1024),
		ooms:      make(chan string, 1024),
	}
	fd, err := archutils.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	m.epollFd = fd
	go m.start()
	return m, nil
}

// Monitor represents a runtime.Process monitor
type Monitor struct {
	m         sync.Mutex
	receivers map[int]interface{}
	exits     chan runtime.Process
	ooms      chan string
	epollFd   int
}

// Exits returns the channel used to notify of a process exit
func (m *Monitor) Exits() chan runtime.Process {
	return m.exits
}

// OOMs returns the channel used to notify of a container exit due to OOM
func (m *Monitor) OOMs() chan string {
	return m.ooms
}

// Monitor adds a process to the list of the one being monitored
func (m *Monitor) Monitor(p runtime.Process) error {
	m.m.Lock()
	defer m.m.Unlock()
	fd := p.ExitFD()
	event := syscall.EpollEvent{
		Fd:     int32(fd),
		Events: syscall.EPOLLHUP,
	}
	if err := archutils.EpollCtl(m.epollFd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return err
	}
	EpollFdCounter.Inc(1)
	m.receivers[fd] = p
	return nil
}

// MonitorOOM adds a container to the list of the ones monitored for OOM
func (m *Monitor) MonitorOOM(c runtime.Container) error {
	m.m.Lock()
	defer m.m.Unlock()
	o, err := c.OOM()
	if err != nil {
		return err
	}
	fd := o.FD()
	event := syscall.EpollEvent{
		Fd:     int32(fd),
		Events: syscall.EPOLLHUP | syscall.EPOLLIN,
	}
	if err := archutils.EpollCtl(m.epollFd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return err
	}
	EpollFdCounter.Inc(1)
	m.receivers[fd] = o
	return nil
}

// Close cleans up resources allocated by NewMonitor()
func (m *Monitor) Close() error {
	return syscall.Close(m.epollFd)
}

func (m *Monitor) start() {
	var events [128]syscall.EpollEvent
	for {
		n, err := archutils.EpollWait(m.epollFd, events[:], -1)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			logrus.WithField("error", err).Fatal("containerd: epoll wait")
		}
		// process events
		for i := 0; i < n; i++ {
			fd := int(events[i].Fd)
			m.m.Lock()
			r := m.receivers[fd]
			switch t := r.(type) {
			case runtime.Process:
				if events[i].Events == syscall.EPOLLHUP {
					delete(m.receivers, fd)
					if err = syscall.EpollCtl(m.epollFd, syscall.EPOLL_CTL_DEL, fd, &syscall.EpollEvent{
						Events: syscall.EPOLLHUP,
						Fd:     int32(fd),
					}); err != nil {
						logrus.WithField("error", err).Error("containerd: epoll remove fd")
					}
					if err := t.Close(); err != nil {
						logrus.WithField("error", err).Error("containerd: close process IO")
					}
					EpollFdCounter.Dec(1)
					m.exits <- t
				}
			case runtime.OOM:
				// always flush the event fd
				t.Flush()
				if t.Removed() {
					delete(m.receivers, fd)
					// epoll will remove the fd from its set after it has been closed
					t.Close()
					EpollFdCounter.Dec(1)
				} else {
					m.ooms <- t.ContainerID()
				}
			}
			m.m.Unlock()
		}
	}
}
