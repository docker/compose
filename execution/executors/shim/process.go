package shim

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/docker/containerd/execution"
	"github.com/docker/containerd/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	runc "github.com/crosbymichael/go-runc"
	starttime "github.com/opencontainers/runc/libcontainer/system"
)

type newProcessOpts struct {
	shimBinary  string
	runtime     string
	runtimeArgs []string
	container   *execution.Container
	exec        bool
	execution.StartProcessOpts
}

func newProcess(ctx context.Context, o newProcessOpts) (*process, error) {
	procStateDir, err := o.container.StateDir().NewProcess(o.ID)
	if err != nil {
		return nil, err
	}

	exitPipe, controlPipe, err := getControlPipes(procStateDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			exitPipe.Close()
			controlPipe.Close()
		}
	}()

	cmd, err := newShim(o, procStateDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	abortCh := make(chan syscall.WaitStatus, 1)
	go func() {
		var shimStatus syscall.WaitStatus
		if err := cmd.Wait(); err != nil {
			shimStatus = execution.UnknownStatusCode
		} else {
			shimStatus = cmd.ProcessState.Sys().(syscall.WaitStatus)
		}
		abortCh <- shimStatus
		close(abortCh)
	}()

	process := &process{
		root:        procStateDir,
		id:          o.ID,
		exitChan:    make(chan struct{}),
		exitPipe:    exitPipe,
		controlPipe: controlPipe,
	}

	pid, stime, status, err := waitForPid(ctx, abortCh, procStateDir)
	if err != nil {
		return nil, err
	}
	process.pid = int64(pid)
	process.status = status
	process.startTime = stime

	return process, nil
}

func loadProcess(root, id string) (*process, error) {
	pid, err := runc.ReadPidFile(filepath.Join(root, pidFilename))
	if err != nil {
		return nil, err
	}

	stime, err := ioutil.ReadFile(filepath.Join(root, startTimeFilename))
	if err != nil {
		return nil, err
	}

	path := filepath.Join(root, exitPipeFilename)
	exitPipe, err := os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			exitPipe.Close()
		}
	}()

	path = filepath.Join(root, controlPipeFilename)
	controlPipe, err := os.OpenFile(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			controlPipe.Close()
		}
	}()

	p := &process{
		root:        root,
		id:          id,
		pid:         int64(pid),
		exitChan:    make(chan struct{}),
		exitPipe:    exitPipe,
		controlPipe: controlPipe,
		startTime:   string(stime),
		// TODO: status may need to be stored on disk to handle
		// Created state for init (i.e. a Start is needed to run the
		// container)
		status: execution.Running,
	}

	markAsStopped := func(p *process) (*process, error) {
		p.setStatus(execution.Stopped)
		return p, nil
	}

	if err = syscall.Kill(pid, 0); err != nil {
		if err == syscall.ESRCH {
			return markAsStopped(p)
		}
		return nil, err
	}

	cstime, err := starttime.GetProcessStartTime(pid)
	if err != nil {
		if os.IsNotExist(err) {
			return markAsStopped(p)
		}
		return nil, err
	}

	if p.startTime != cstime {
		return markAsStopped(p)
	}

	return p, nil
}

type process struct {
	root        string
	id          string
	pid         int64
	exitChan    chan struct{}
	exitPipe    *os.File
	controlPipe *os.File
	startTime   string
	status      execution.Status
	ctx         context.Context
	mu          sync.Mutex
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Pid() int64 {
	return p.pid
}

func (p *process) Wait() (uint32, error) {
	<-p.exitChan

	log.G(p.ctx).WithFields(logrus.Fields{"process-id": p.ID(), "pid": p.pid}).
		Debugf("wait is over")

	// Cleanup those fds
	p.exitPipe.Close()
	p.controlPipe.Close()

	// If the container process is still alive, it means the shim crashed
	// and the child process had updated it PDEATHSIG to something
	// else than SIGKILL. Or that epollCtl failed
	if p.isAlive() {
		err := syscall.Kill(int(p.pid), syscall.SIGKILL)
		if err != nil {
			return execution.UnknownStatusCode, errors.Wrap(err, "failed to kill process")
		}

		return uint32(128 + int(syscall.SIGKILL)), nil
	}

	data, err := ioutil.ReadFile(filepath.Join(p.root, exitStatusFilename))
	if err != nil {
		return execution.UnknownStatusCode, errors.Wrap(err, "failed to read process exit status")
	}

	if len(data) == 0 {
		return execution.UnknownStatusCode, errors.New(execution.ErrProcessNotExited.Error())
	}

	status, err := strconv.Atoi(string(data))
	if err != nil {
		return execution.UnknownStatusCode, errors.Wrapf(err, "failed to parse exit status")
	}

	p.setStatus(execution.Stopped)
	return uint32(status), nil
}

func (p *process) Signal(sig os.Signal) error {
	err := syscall.Kill(int(p.pid), sig.(syscall.Signal))
	if err != nil {
		return errors.Wrap(err, "failed to signal process")
	}
	return nil
}

func (p *process) Status() execution.Status {
	p.mu.Lock()
	s := p.status
	p.mu.Unlock()
	return s
}

func (p *process) setStatus(s execution.Status) {
	p.mu.Lock()
	p.status = s
	p.mu.Unlock()
}

func (p *process) isAlive() bool {
	if err := syscall.Kill(int(p.pid), 0); err != nil {
		if err == syscall.ESRCH {
			return false
		}
		log.G(p.ctx).WithFields(logrus.Fields{"process-id": p.ID(), "pid": p.pid}).
			Warnf("kill(0) failed: %v", err)
		return false
	}

	// check that we have the same startttime
	stime, err := starttime.GetProcessStartTime(int(p.pid))
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		log.G(p.ctx).WithFields(logrus.Fields{"process-id": p.ID(), "pid": p.pid}).
			Warnf("failed to get process start time: %v", err)
		return false
	}

	if p.startTime != stime {
		return false
	}

	return true
}

func waitForPid(ctx context.Context, abortCh chan syscall.WaitStatus, root string) (pid int, stime string, status execution.Status, err error) {
	status = execution.Unknown
	for {
		select {
		case <-ctx.Done():
			return
		case wait := <-abortCh:
			if wait.Signaled() {
				err = errors.Errorf("shim died prematurarily: %v", wait.Signal())
				return
			}
			err = errors.Errorf("shim exited prematurarily with exit code %v", wait.ExitStatus())
			return
		default:
		}
		pid, err = runc.ReadPidFile(filepath.Join(root, pidFilename))
		if err == nil {
			break
		} else if !os.IsNotExist(err) {
			return
		}
	}
	status = execution.Created
	stime, err = starttime.GetProcessStartTime(pid)
	switch {
	case os.IsNotExist(err):
		status = execution.Stopped
	case err != nil:
		return
	default:
		var b []byte
		path := filepath.Join(root, startTimeFilename)
		b, err = ioutil.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			err = ioutil.WriteFile(path, []byte(stime), 0600)
			if err != nil {
				return
			}
		case err != nil:
			err = errors.Wrapf(err, "failed to get start time for pid %d", pid)
			return
		case string(b) != stime:
			status = execution.Stopped
		}
	}

	return pid, stime, status, nil
}

func newShim(o newProcessOpts, workDir string) (*exec.Cmd, error) {
	cmd := exec.Command(o.shimBinary, o.container.ID(), o.container.Bundle(), o.runtime)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	state := processState{
		Process:        o.Spec,
		Exec:           o.exec,
		Stdin:          o.Stdin,
		Stdout:         o.Stdout,
		Stderr:         o.Stderr,
		RuntimeArgs:    o.runtimeArgs,
		NoPivotRoot:    false,
		CheckpointPath: "",
		RootUID:        int(o.Spec.User.UID),
		RootGID:        int(o.Spec.User.GID),
	}

	f, err := os.Create(filepath.Join(workDir, "process.json"))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create shim's process.json for container %s", o.container.ID())
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(state); err != nil {
		return nil, errors.Wrapf(err, "failed to create shim's processState for container %s", o.container.ID())
	}

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "failed to start shim for container %s", o.container.ID())
	}

	return cmd, nil
}

func getControlPipes(root string) (exitPipe *os.File, controlPipe *os.File, err error) {
	path := filepath.Join(root, exitPipeFilename)
	if err = unix.Mkfifo(path, 0700); err != nil {
		return exitPipe, controlPipe, errors.Wrap(err, "failed to create shim exit fifo")
	}
	if exitPipe, err = os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0); err != nil {
		return exitPipe, controlPipe, errors.Wrap(err, "failed to open shim exit fifo")
	}

	path = filepath.Join(root, controlPipeFilename)
	if err = unix.Mkfifo(path, 0700); err != nil {
		return exitPipe, controlPipe, errors.Wrap(err, "failed to create shim control fifo")
	}
	if controlPipe, err = os.OpenFile(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0); err != nil {
		return exitPipe, controlPipe, errors.Wrap(err, "failed to open shim control fifo")
	}

	return exitPipe, controlPipe, nil
}
