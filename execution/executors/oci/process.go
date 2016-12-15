package oci

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/crosbymichael/go-runc"
	"github.com/docker/containerd/execution"
	starttime "github.com/opencontainers/runc/libcontainer/system"
)

func newProcess(c *execution.Container, id, stateDir string) (execution.Process, error) {
	pid, err := runc.ReadPidFile(filepath.Join(stateDir, PidFilename))
	if err != nil {
		return nil, err
	}
	status := execution.Running
	if err := syscall.Kill(pid, 0); err != nil {
		if err == syscall.ESRCH {
			status = execution.Stopped
		} else {
			return nil, err
		}
	}
	if status == execution.Running {
		stime, err := starttime.GetProcessStartTime(pid)
		switch {
		case os.IsNotExist(err):
			status = execution.Stopped
		case err != nil:
			return nil, err
		default:
			b, err := ioutil.ReadFile(filepath.Join(stateDir, StartTimeFilename))
			switch {
			case os.IsNotExist(err):
				err = ioutil.WriteFile(filepath.Join(stateDir, StartTimeFilename), []byte(stime), 0600)
				if err != nil {
					return nil, err
				}
			case err != nil:
				return nil, err
			case string(b) != stime:
				status = execution.Stopped
			}
		}
	}
	return &process{
		c:      c,
		id:     id,
		pid:    pid,
		status: status,
	}, nil
}

type process struct {
	c      *execution.Container
	id     string
	pid    int
	status execution.Status
}

func (p *process) Container() *execution.Container {
	return p.c
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Pid() int64 {
	return int64(p.pid)
}

func (p *process) Wait() (uint32, error) {
	if p.status == execution.Running {
		var wstatus syscall.WaitStatus
		_, err := syscall.Wait4(p.pid, &wstatus, 0, nil)
		if err != nil {
			return 255, nil
		}
		// TODO: implement kill-all if we are the init pid
		p.status = execution.Stopped
		return uint32(wstatus.ExitStatus()), nil
	}

	return 255, execution.ErrProcessNotFound
}

func (p *process) Signal(s os.Signal) error {
	if p.status == execution.Running {
		sig, ok := s.(syscall.Signal)
		if !ok {
			return fmt.Errorf("invalid signal %v", s)
		}
		return syscall.Kill(p.pid, sig)
	}
	return execution.ErrProcessNotFound
}

func (p *process) Status() execution.Status {
	return p.status
}
