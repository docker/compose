package shim

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerkit"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

var (
	ErrContainerStartTimeout = errors.New("shim: container did not start before the specified timeout")
	ErrContainerNotStarted   = errors.New("shim: container not started")
	ErrProcessNotExited      = errors.New("containerd: process has not exited")
	ErrShimExited            = errors.New("containerd: shim exited before container process was started")
	errInvalidPidInt         = errors.New("shim: process pid is invalid")
)

const UnknownStatus = 255

func newProcess(root string, noPivotRoot bool, checkpoint string, c *containerkit.Container, cmd *exec.Cmd) (*process, error) {
	if err := os.Mkdir(root, 0711); err != nil {
		return nil, err
	}
	var (
		spec                  = c.Spec()
		stdin, stdout, stderr string
	)
	uid, gid, err := getRootIDs(spec)
	if err != nil {
		return nil, err
	}
	for _, t := range []struct {
		path *string
		v    interface{}
	}{
		{
			path: &stdin,
			v:    c.Stdin,
		},
		{
			path: &stdout,
			v:    c.Stdout,
		},
		{
			path: &stderr,
			v:    c.Stderr,
		},
	} {
		p, err := getFifoPath(t.v)
		if err != nil {
			return nil, err
		}
		*t.path = p
	}
	p := &process{
		root:        root,
		cmd:         cmd,
		done:        make(chan struct{}),
		spec:        spec.Process,
		exec:        false,
		rootUid:     uid,
		rootGid:     gid,
		noPivotRoot: noPivotRoot,
		checkpoint:  checkpoint,
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
	}
	f, err := os.Create(filepath.Join(root, "process.json"))
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(f).Encode(p)
	f.Close()
	if err != nil {
		return nil, err
	}
	exit, err := getExitPipe(filepath.Join(root, "exit"))
	if err != nil {
		return nil, err
	}
	control, err := getControlPipe(filepath.Join(root, "control"))
	if err != nil {
		return nil, err
	}
	p.exit, p.control = exit, control
	return p, nil
}

type process struct {
	root      string
	cmd       *exec.Cmd
	done      chan struct{}
	success   bool
	startTime string
	mu        sync.Mutex
	pid       int
	exit      *os.File
	control   *os.File

	spec        specs.Process
	noPivotRoot bool
	exec        bool
	rootUid     int
	rootGid     int
	checkpoint  string
	stdin       string
	stdout      string
	stderr      string
}

type processState struct {
	specs.Process
	Exec        bool     `json:"exec"`
	RootUID     int      `json:"rootUID"`
	RootGID     int      `json:"rootGID"`
	Checkpoint  string   `json:"checkpoint"`
	NoPivotRoot bool     `json:"noPivotRoot"`
	RuntimeArgs []string `json:"runtimeArgs"`
	Root        string   `json:"root"`
	StartTime   string   `json:"startTime"`
	// Stdin fifo filepath
	Stdin string `json:"stdin"`
	// Stdout fifo filepath
	Stdout string `json:"stdout"`
	// Stderr fifo filepath
	Stderr string `json:"stderr"`
}

func (p *process) MarshalJSON() ([]byte, error) {
	ps := processState{
		Process:     p.spec,
		NoPivotRoot: p.noPivotRoot,
		Checkpoint:  p.checkpoint,
		RootUID:     p.rootUid,
		RootGID:     p.rootGid,
		Exec:        p.exec,
		Stdin:       p.stdin,
		Stdout:      p.stdout,
		Stderr:      p.stderr,
		Root:        p.root,
		StartTime:   p.startTime,
	}
	return json.Marshal(ps)
}

func (p *process) UnmarshalJSON(b []byte) error {
	var ps processState
	if err := json.Unmarshal(b, &ps); err != nil {
		return err
	}
	p.spec = ps.Process
	p.noPivotRoot = ps.NoPivotRoot
	p.rootGid = ps.RootGID
	p.rootUid = ps.RootUID
	p.checkpoint = ps.Checkpoint
	p.exec = ps.Exec
	p.stdin = ps.Stdin
	p.stdout = ps.Stdout
	p.stderr = ps.Stderr
	p.root = ps.Root
	p.startTime = ps.StartTime
	pid, err := readPid(filepath.Join(p.root, "pid"))
	if err != nil {
		return err
	}
	p.pid = pid
	exit, err := getExitPipe(filepath.Join(p.root, "exit"))
	if err != nil {
		return err
	}
	control, err := getControlPipe(filepath.Join(p.root, "control"))
	if err != nil {
		return err
	}
	p.exit, p.control = exit, control
	return nil
}

func (p *process) Pid() int {
	return p.pid
}

func (p *process) FD() int {
	return int(p.exit.Fd())
}

func (p *process) Wait() (rst uint32, rerr error) {
	<-p.done
	data, err := ioutil.ReadFile(filepath.Join(p.root, "exitStatus"))
	defer func() {
		if rerr != nil {
			rst, rerr = p.handleSigkilledShim(rst, rerr)
		}
	}()
	if err != nil {
		if os.IsNotExist(err) {
			return UnknownStatus, ErrProcessNotExited
		}
		return UnknownStatus, err
	}
	if len(data) == 0 {
		return UnknownStatus, ErrProcessNotExited
	}
	i, err := strconv.ParseUint(string(data), 10, 32)
	return uint32(i), err
}

func (p *process) Signal(s os.Signal) error {
	_, err := fmt.Fprintf(p.control, "%d %d %d\n", 2, s, 0)
	return err
}

// same checks if the process is the same process originally launched
func (p *process) same() (bool, error) {
	/// for backwards compat assume true if it is not set
	if p.startTime == "" {
		return true, nil
	}
	pid, err := readPid(filepath.Join(p.root, "pid"))
	if err != nil {
		return false, nil
	}
	started, err := readProcessStartTime(pid)
	if err != nil {
		return false, err
	}
	return p.startTime == started, nil
}

func (p *process) checkExited() {
	err := p.cmd.Wait()
	if err == nil {
		p.success = true
	}
	if same, _ := p.same(); same && p.hasPid() {
		// The process changed its PR_SET_PDEATHSIG, so force kill it
		logrus.Infof("containerd: (pid %v) has become an orphan, killing it", p.pid)
		if err := unix.Kill(p.pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			logrus.Errorf("containerd: unable to SIGKILL (pid %v): %v", p.pid, err)
			close(p.done)
			return
		}
		// wait for the container process to exit
		for {
			if err := unix.Kill(p.pid, 0); err != nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	close(p.done)
}

func (p *process) hasPid() bool {
	p.mu.Lock()
	r := p.pid > 0
	p.mu.Unlock()
	return r
}

func (p *process) setPid(pid int) {
	p.mu.Lock()
	p.pid = pid
	p.mu.Unlock()
}

type pidResponse struct {
	pid int
	err error
}

func (p *process) waitForCreate(timeout time.Duration) error {
	r := make(chan pidResponse, 1)
	go p.readContainerPid(r)

	select {
	case resp := <-r:
		if resp.err != nil {
			return resp.err
		}
		p.setPid(resp.pid)
		started, err := readProcessStartTime(resp.pid)
		if err != nil {
			logrus.Warnf("shim: unable to save starttime: %v", err)
		}
		p.startTime = started
		f, err := os.Create(filepath.Join(p.root, "process.json"))
		if err != nil {
			logrus.Warnf("shim: unable to save starttime: %v", err)
			return nil
		}
		defer f.Close()
		if err := json.NewEncoder(f).Encode(p); err != nil {
			logrus.Warnf("shim: unable to save starttime: %v", err)
		}
		return nil
	case <-time.After(timeout):
		p.cmd.Process.Kill()
		p.cmd.Wait()
		return ErrContainerStartTimeout
	}
}

func (p *process) readContainerPid(r chan pidResponse) {
	pidFile := filepath.Join(p.root, "pid")
	for {
		pid, err := readPid(pidFile)
		if err != nil {
			if os.IsNotExist(err) || err == errInvalidPidInt {
				if serr := checkErrorLogs(p.cmd,
					filepath.Join(p.root, "shim-log.json"),
					filepath.Join(p.root, "log.json")); serr != nil && !os.IsNotExist(serr) {
					r <- pidResponse{
						err: serr,
					}
					break
				}
				time.Sleep(15 * time.Millisecond)
				continue
			}
			r <- pidResponse{
				err: err,
			}
			break
		}
		r <- pidResponse{
			pid: pid,
		}
		break
	}
}

func (p *process) handleSigkilledShim(rst uint32, rerr error) (uint32, error) {
	if err := unix.Kill(p.pid, 0); err == syscall.ESRCH {
		logrus.Warnf("containerd: (pid %d) does not exist", p.pid)
		// The process died while containerd was down (probably of
		// SIGKILL, but no way to be sure)
		return UnknownStatus, writeExitStatus(filepath.Join(p.root, "exitStatus"), UnknownStatus)
	}

	// If it's not the same process, just mark it stopped and set
	// the status to the UnknownStatus value (i.e. 255)
	if same, _ := p.same(); !same {
		// Create the file so we get the exit event generated once monitor kicks in
		// without having to go through all this process again
		return UnknownStatus, writeExitStatus(filepath.Join(p.root, "exitStatus"), UnknownStatus)
	}
	ppid, err := readProcStatField(p.pid, 4)
	if err != nil {
		return rst, fmt.Errorf("could not check process ppid: %v (%v)", err, rerr)
	}
	if ppid == "1" {
		if err := unix.Kill(p.pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return UnknownStatus, fmt.Errorf(
				"containerd: unable to SIGKILL (pid %v): %v", p.pid, err)
		}
		// wait for the process to die
		for {
			if err := unix.Kill(p.pid, 0); err == syscall.ESRCH {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		// Create the file so we get the exit event generated once monitor kicks in
		// without having to go through all this process again
		status := 128 + uint32(syscall.SIGKILL)
		return status, writeExitStatus(filepath.Join(p.root, "exitStatus"), status)
	}
	return rst, rerr
}

func checkErrorLogs(cmd *exec.Cmd, shimLogPath, runtimeLogPath string) error {
	alive, err := isAlive(cmd)
	if err != nil {
		return err
	}
	if !alive {
		// runc could have failed to run the container so lets get the error
		// out of the logs or the shim could have encountered an error
		messages, err := readLogMessages(shimLogPath)
		if err != nil {
			return err
		}
		for _, m := range messages {
			if m.Level == "error" {
				return fmt.Errorf("shim error: %v", m.Msg)
			}
		}
		// no errors reported back from shim, check for runc/runtime errors
		messages, err = readLogMessages(runtimeLogPath)
		if err != nil {
			if os.IsNotExist(err) {
				err = ErrContainerNotStarted
			}
			return err
		}
		for _, m := range messages {
			if m.Level == "error" {
				return fmt.Errorf("oci runtime error: %v", m.Msg)
			}
		}
		return ErrContainerNotStarted
	}
	return nil
}

func readProcessStartTime(pid int) (string, error) {
	return readProcStatField(pid, 22)
}

func readProcStatField(pid int, field int) (string, error) {
	data, err := ioutil.ReadFile(filepath.Join(string(filepath.Separator), "proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	if field > 2 {
		// First, split out the name since he could contains spaces.
		parts := strings.Split(string(data), ") ")
		// Now split out the rest, we end up with 2 fields less
		parts = strings.Split(parts[1], " ")
		return parts[field-2-1], nil // field count start at 1 in manual
	}
	parts := strings.Split(string(data), " (")
	if field == 1 {
		return parts[0], nil
	}
	return strings.Split(parts[1], ") ")[0], nil
}

func readPid(pidFile string) (int, error) {
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return -1, err
	}
	i, err := strconv.Atoi(string(data))
	if err != nil {
		return -1, errInvalidPidInt
	}
	return i, nil
}

// isAlive checks if the shim that launched the container is still alive
func isAlive(cmd *exec.Cmd) (bool, error) {
	if _, err := syscall.Wait4(cmd.Process.Pid, nil, syscall.WNOHANG, nil); err == nil {
		return true, nil
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		if err == syscall.ESRCH {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type message struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

func readLogMessages(path string) ([]message, error) {
	var out []message
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var m message
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
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

func writeExitStatus(path string, status uint32) error {
	return ioutil.WriteFile(path, []byte(fmt.Sprintf("%u", status)), 0644)
}
