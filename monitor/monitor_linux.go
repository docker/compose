package monitor

import (
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/epoll"
)

type Monitorable interface {
	FD() int
}

type Flusher interface {
	Flush() error
}

// New returns a new process monitor that emits events whenever the
// state of the fd refering to a process changes
func New() (*Monitor, error) {
	fd, err := epoll.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &Monitor{
		epollFd:   fd,
		receivers: make(map[int]Monitorable),
		events:    make(chan Monitorable, 1024),
	}, nil
}

type Monitor struct {
	m         sync.Mutex
	receivers map[int]Monitorable
	events    chan Monitorable
	epollFd   int
}

// Events returns a chan that receives a Monitorable when it's FD changes state
func (m *Monitor) Events() chan Monitorable {
	return m.events
}

// Add adds a process to the list of the one being monitored
func (m *Monitor) Add(ma Monitorable) error {
	m.m.Lock()
	defer m.m.Unlock()
	fd := ma.FD()
	event := syscall.EpollEvent{
		Fd:     int32(fd),
		Events: syscall.EPOLLHUP,
	}
	if err := epoll.EpollCtl(m.epollFd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return err
	}
	m.receivers[fd] = ma
	return nil
}

// Remove deletes the Monitorable type from the monitor so that
// no other events are generated
func (m *Monitor) Remove(ma Monitorable) error {
	m.m.Lock()
	defer m.m.Unlock()
	fd := ma.FD()
	delete(m.receivers, fd)
	return syscall.EpollCtl(m.epollFd, syscall.EPOLL_CTL_DEL, fd, &syscall.EpollEvent{
		Events: syscall.EPOLLHUP,
		Fd:     int32(fd),
	})
}

// Close cleans up resources allocated to the Monitor
func (m *Monitor) Close() error {
	return syscall.Close(m.epollFd)
}

func (m *Monitor) Run() {
	var events [128]syscall.EpollEvent
	for {
		n, err := epoll.EpollWait(m.epollFd, events[:], -1)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			logrus.WithField("error", err).Fatal("containerd: epoll wait")
		}
		for i := 0; i < n; i++ {
			fd := int(events[i].Fd)
			m.m.Lock()
			r := m.receivers[fd]
			m.m.Unlock()
			if f, ok := r.(Flusher); ok {
				if err := f.Flush(); err != nil {
					logrus.WithField("error", err).Fatal("containerd: flush event FD")
				}
			}
			m.events <- r
		}
	}
}
