package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/specs"
	"golang.org/x/sys/unix"
)

// Process holds the operation allowed on a container's process
type Process interface {
	io.Closer

	// ID of the process.
	// This is either "init" when it is the container's init process or
	// it is a user provided id for the process similar to the container id
	ID() string
	// Start unblocks the associated container init process.
	// This should only be called on the process with ID "init"
	Start() error
	CloseStdin() error
	Resize(int, int) error
	// ExitFD returns the fd the provides an event when the process exits
	ExitFD() int
	// ExitStatus returns the exit status of the process or an error if it
	// has not exited
	ExitStatus() (int, error)
	// Spec returns the process spec that created the process
	Spec() specs.ProcessSpec
	// Signal sends the provided signal to the process
	Signal(os.Signal) error
	// Container returns the container that the process belongs to
	Container() Container
	// Stdio of the container
	Stdio() Stdio
	// SystemPid is the pid on the system
	SystemPid() int
	// State returns if the process is running or not
	State() State
	// Wait reaps the shim process if avaliable
	Wait()
}

type processConfig struct {
	id          string
	root        string
	processSpec specs.ProcessSpec
	spec        *specs.Spec
	c           *container
	stdio       Stdio
	exec        bool
	checkpoint  string
}

func newProcess(config *processConfig) (*process, error) {
	p := &process{
		root:      config.root,
		id:        config.id,
		container: config.c,
		spec:      config.processSpec,
		stdio:     config.stdio,
		cmdDoneCh: make(chan struct{}),
		state:     Running,
	}
	uid, gid, err := getRootIDs(config.spec)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(config.root, "process.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ps := ProcessState{
		ProcessSpec: config.processSpec,
		Exec:        config.exec,
		PlatformProcessState: PlatformProcessState{
			Checkpoint: config.checkpoint,
			RootUID:    uid,
			RootGID:    gid,
		},
		Stdin:       config.stdio.Stdin,
		Stdout:      config.stdio.Stdout,
		Stderr:      config.stdio.Stderr,
		RuntimeArgs: config.c.runtimeArgs,
		NoPivotRoot: config.c.noPivotRoot,
	}

	if err := json.NewEncoder(f).Encode(ps); err != nil {
		return nil, err
	}
	exit, err := getExitPipe(filepath.Join(config.root, ExitFile))
	if err != nil {
		return nil, err
	}
	control, err := getControlPipe(filepath.Join(config.root, ControlFile))
	if err != nil {
		return nil, err
	}
	p.exitPipe = exit
	p.controlPipe = control
	return p, nil
}

func loadProcess(root, id string, c *container, s *ProcessState) (*process, error) {
	p := &process{
		root:      root,
		id:        id,
		container: c,
		spec:      s.ProcessSpec,
		stdio: Stdio{
			Stdin:  s.Stdin,
			Stdout: s.Stdout,
			Stderr: s.Stderr,
		},
		state: Stopped,
	}
	if _, err := p.getPidFromFile(); err != nil {
		return nil, err
	}
	if _, err := p.ExitStatus(); err != nil {
		if err == ErrProcessNotExited {
			exit, err := getExitPipe(filepath.Join(root, ExitFile))
			if err != nil {
				return nil, err
			}
			p.exitPipe = exit

			control, err := getControlPipe(filepath.Join(root, ControlFile))
			if err != nil {
				return nil, err
			}
			p.controlPipe = control

			p.state = Running
			return p, nil
		}
		return nil, err
	}
	return p, nil
}

type process struct {
	root        string
	id          string
	pid         int
	exitPipe    *os.File
	controlPipe *os.File
	container   *container
	spec        specs.ProcessSpec
	stdio       Stdio
	cmd         *exec.Cmd
	cmdSuccess  bool
	cmdDoneCh   chan struct{}
	state       State
	stateLock   sync.Mutex
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Container() Container {
	return p.container
}

func (p *process) SystemPid() int {
	return p.pid
}

// ExitFD returns the fd of the exit pipe
func (p *process) ExitFD() int {
	return int(p.exitPipe.Fd())
}

func (p *process) CloseStdin() error {
	_, err := fmt.Fprintf(p.controlPipe, "%d %d %d\n", 0, 0, 0)
	return err
}

func (p *process) Resize(w, h int) error {
	_, err := fmt.Fprintf(p.controlPipe, "%d %d %d\n", 1, w, h)
	return err
}

func (p *process) handleSigkilledShim(rst int, rerr error) (int, error) {
	if rerr == nil || p.cmd == nil || p.cmd.Process == nil {
		return rst, rerr
	}

	// Possible that the shim was SIGKILLED
	e := unix.Kill(p.cmd.Process.Pid, 0)
	if e != syscall.ESRCH {
		return rst, rerr
	}

	// Ensure we got the shim ProcessState
	<-p.cmdDoneCh

	shimStatus := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if shimStatus.Signaled() && shimStatus.Signal() == syscall.SIGKILL {
		logrus.Debugf("containerd: ExitStatus(container: %s, process: %s): shim was SIGKILL'ed reaping its child with pid %d", p.container.id, p.id, p.pid)

		var (
			status unix.WaitStatus
			rusage unix.Rusage
			wpid   int
		)

		for wpid == 0 {
			wpid, e = unix.Wait4(p.pid, &status, unix.WNOHANG, &rusage)
			if e != nil {
				logrus.Debugf("containerd: ExitStatus(container: %s, process: %s): Wait4(%d): %v", p.container.id, p.id, p.pid, rerr)
				return rst, rerr
			}
		}

		if wpid == p.pid {
			rerr = nil
			rst = 128 + int(shimStatus.Signal())
		} else {
			logrus.Errorf("containerd: ExitStatus(container: %s, process: %s): unexpected returned pid from wait4 %v (expected %v)", p.container.id, p.id, wpid, p.pid)
		}

		p.stateLock.Lock()
		p.state = Stopped
		p.stateLock.Unlock()
	}

	return rst, rerr
}

func (p *process) ExitStatus() (rst int, rerr error) {
	data, err := ioutil.ReadFile(filepath.Join(p.root, ExitStatusFile))
	defer func() {
		rst, rerr = p.handleSigkilledShim(rst, rerr)
	}()
	if err != nil {
		if os.IsNotExist(err) {
			return -1, ErrProcessNotExited
		}
		return -1, err
	}
	if len(data) == 0 {
		return -1, ErrProcessNotExited
	}
	p.stateLock.Lock()
	p.state = Stopped
	p.stateLock.Unlock()
	return strconv.Atoi(string(data))
}

func (p *process) Spec() specs.ProcessSpec {
	return p.spec
}

func (p *process) Stdio() Stdio {
	return p.stdio
}

// Close closes any open files and/or resouces on the process
func (p *process) Close() error {
	err := p.exitPipe.Close()
	if cerr := p.controlPipe.Close(); err == nil {
		err = cerr
	}
	return err
}

func (p *process) State() State {
	p.stateLock.Lock()
	defer p.stateLock.Unlock()
	return p.state
}

func (p *process) getPidFromFile() (int, error) {
	data, err := ioutil.ReadFile(filepath.Join(p.root, "pid"))
	if err != nil {
		return -1, err
	}
	i, err := strconv.Atoi(string(data))
	if err != nil {
		return -1, errInvalidPidInt
	}
	p.pid = i
	return i, nil
}

// Wait will reap the shim process
func (p *process) Wait() {
	if p.cmdDoneCh != nil {
		<-p.cmdDoneCh
	}
}

func getExitPipe(path string) (*os.File, error) {
	if err := unix.Mkfifo(path, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	// add NONBLOCK in case the other side has already closed or else
	// this function would never return
	return os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func getControlPipe(path string) (*os.File, error) {
	if err := unix.Mkfifo(path, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	return os.OpenFile(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
}

// Signal sends the provided signal to the process
func (p *process) Signal(s os.Signal) error {
	return syscall.Kill(p.pid, s.(syscall.Signal))
}

// Start unblocks the associated container init process.
// This should only be called on the process with ID "init"
func (p *process) Start() error {
	if p.ID() == InitProcessID {
		var (
			errC = make(chan error, 1)
			args = append(p.container.runtimeArgs, "start", p.container.id)
			cmd  = exec.Command(p.container.runtime, args...)
		)
		go func() {
			out, err := cmd.CombinedOutput()
			if err != nil {
				errC <- fmt.Errorf("%s: %q", err.Error(), out)
			}
			errC <- nil
		}()
		select {
		case err := <-errC:
			if err != nil {
				return err
			}
		case <-p.cmdDoneCh:
			if !p.cmdSuccess {
				if cmd.Process != nil {
					cmd.Process.Kill()
				}
				cmd.Wait()
				return ErrShimExited
			}
			err := <-errC
			if err != nil {
				return err
			}
		}
	}
	return nil
}
