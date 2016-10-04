package process

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
	"golang.org/x/sys/unix"
)

var errInvalidPidInt = errors.New("containerd: process pid is invalid")

type process struct {
	name         string
	root         string
	cmd          *exec.Cmd
	done         chan struct{}
	success      bool
	startTime    string
	mu           sync.Mutex
	containerPid int
	timeout      time.Duration
}

// same checks if the process is the same process originally launched
func (p *process) same() (bool, error) {
	/// for backwards compat assume true if it is not set
	if p.startTime == "" {
		return true, nil
	}
	pid, err := p.readContainerPid()
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
		logrus.Infof("containerd: %s:%s (pid %v) has become an orphan, killing it", p.container.id, p.namae, p.containerPid)
		if err := unix.Kill(p.containerPid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			logrus.Errorf("containerd: unable to SIGKILL %s:%s (pid %v): %v", p.container.id, p.name, p.containerPid, err)
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
	r := p.containerPid > 0
	p.mu.Unlock()
	return r
}

func (p *process) setPid(pid int) {
	p.mu.Lock()
	p.containerPid = pid
	p.mu.Unlock()
}

type pidResponse struct {
	pid int
	err error
}

func (p *process) waitForCreate() error {
	r := make(chan pidResponse, 1)
	go readContainerPid(wc)

	select {
	case resp := <-r:
		if resp.err != nil {
			return resp.err
		}
		p.setPid(resp.pid)
		started, err := readProcessStartTime(resp.pid)
		if err != nil {
			logrus.Warnf("containerd: unable to save %s:%s starttime: %v", p.container.id, p.id, err)
		}
		// TODO: save start time to disk or process state file
		p.startTime = started
		return nil
	case <-time.After(c.timeout):
		p.cmd.Process.Kill()
		p.cmd.Wait()
		return ErrContainerStartTimeout
	}
}

func readContainerPid(r chan pidResponse, pidFile string) {
	for {
		pid, err := readContainerPid(pidFile)
		if err != nil {
			if os.IsNotExist(err) || err == errInvalidPidInt {
				if serr := checkErrorLogs(); serr != nil {
					r <- pidResponse{
						err: err,
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

func readContainerPid(pidFile string) (int, error) {
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return -1, nil
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
