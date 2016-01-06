package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/opencontainers/specs"
)

func newProcess(root, id string, c *container, s specs.Process) (*process, error) {
	p := &process{
		root:      root,
		id:        id,
		container: c,
		spec:      s,
	}
	// create fifo's for the process
	for name, fd := range map[string]*string{
		"stdin":  &p.stdin,
		"stdout": &p.stdout,
		"stderr": &p.stderr,
	} {
		path := filepath.Join(root, name)
		if err := syscall.Mkfifo(path, 0755); err != nil && !os.IsExist(err) {
			return nil, err
		}
		*fd = path
	}
	exit, err := getExitPipe(filepath.Join(root, ExitFile))
	if err != nil {
		return nil, err
	}
	p.exitPipe = exit
	return p, nil
}

func loadProcess(root, id string, c *container, s specs.Process) (*process, error) {
	p := &process{
		root:      root,
		id:        id,
		container: c,
		spec:      s,
		stdin:     filepath.Join(root, "stdin"),
		stdout:    filepath.Join(root, "stdout"),
		stderr:    filepath.Join(root, "stderr"),
	}
	if _, err := p.ExitStatus(); err != nil {
		if err == ErrProcessNotExited {
			exit, err := getExitPipe(filepath.Join(root, ExitFile))
			if err != nil {
				return nil, err
			}
			p.exitPipe = exit
			return p, nil
		}
		return nil, err
	}
	return nil, ErrProcessExited
}

func getExitPipe(path string) (*os.File, error) {
	if err := syscall.Mkfifo(path, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	// add NONBLOCK in case the other side has already closed or else
	// this function would never return
	return os.OpenFile(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
}

type process struct {
	root string
	id   string
	// stdio fifos
	stdin  string
	stdout string
	stderr string

	exitPipe  *os.File
	container *container
	spec      specs.Process
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Container() Container {
	return p.container
}

// ExitFD returns the fd of the exit pipe
func (p *process) ExitFD() int {
	return int(p.exitPipe.Fd())
}

func (p *process) ExitStatus() (int, error) {
	data, err := ioutil.ReadFile(filepath.Join(p.root, ExitStatusFile))
	if err != nil {
		if os.IsNotExist(err) {
			return -1, ErrProcessNotExited
		}
		return -1, err
	}
	if len(data) == 0 {
		return -1, ErrProcessNotExited
	}
	i, err := strconv.Atoi(string(data))
	if err != nil {
		return -1, err
	}
	return i, nil
}

// Signal sends the provided signal to the process
func (p *process) Signal(s os.Signal) error {
	return errNotImplemented
}

func (p *process) Spec() specs.Process {
	return p.spec
}

func (p *process) Stdin() string {
	return p.stdin
}

func (p *process) Stdout() string {
	return p.stdout
}

func (p *process) Stderr() string {
	return p.stderr
}

// Close closes any open files and/or resouces on the process
func (p *process) Close() error {
	return p.exitPipe.Close()
}
