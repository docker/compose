package supervisor

import (
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

func NewMonitor() (*Monitor, error) {
	m := &Monitor{
		processes: make(map[int]runtime.Process),
		exits:     make(chan runtime.Process, 1024),
	}
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	m.epollFd = fd
	go m.start()
	return m, nil
}

type Monitor struct {
	m         sync.Mutex
	processes map[int]runtime.Process
	exits     chan runtime.Process
	epollFd   int
}

func (m *Monitor) Exits() chan runtime.Process {
	return m.exits
}

func (m *Monitor) Monitor(p runtime.Process) error {
	m.m.Lock()
	defer m.m.Unlock()
	fd := p.ExitFD()
	event := syscall.EpollEvent{
		Fd:     int32(fd),
		Events: syscall.EPOLLHUP,
	}
	if err := syscall.EpollCtl(m.epollFd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return err
	}
	EpollFdCounter.Inc(1)
	m.processes[fd] = p
	return nil
}

func (m *Monitor) Close() error {
	return syscall.Close(m.epollFd)
}

func (m *Monitor) start() {
	var events [128]syscall.EpollEvent
	for {
		n, err := syscall.EpollWait(m.epollFd, events[:], -1)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			logrus.WithField("error", err).Fatal("containerd: epoll wait")
		}
		// process events
		for i := 0; i < n; i++ {
			if events[i].Events == syscall.EPOLLHUP {
				fd := int(events[i].Fd)
				m.m.Lock()
				proc := m.processes[fd]
				delete(m.processes, fd)
				if err = syscall.EpollCtl(m.epollFd, syscall.EPOLL_CTL_DEL, fd, &syscall.EpollEvent{
					Events: syscall.EPOLLHUP,
					Fd:     int32(fd),
				}); err != nil {
					logrus.WithField("error", err).Fatal("containerd: epoll remove fd")
				}
				EpollFdCounter.Dec(1)
				if err := proc.Close(); err != nil {
					logrus.WithField("error", err).Error("containerd: close process IO")
				}
				m.m.Unlock()
				m.exits <- proc
			}
		}
	}
}
