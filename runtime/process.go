package runtime

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/opencontainers/specs"
)

type Process interface {
	io.Closer

	// ID of the process.
	// This is either "init" when it is the container's init process or
	// it is a user provided id for the process similar to the container id
	ID() string
	// Stdin returns the path the the processes stdin fifo
	Stdin() string
	// Stdout returns the path the the processes stdout fifo
	Stdout() string
	// Stderr returns the path the the processes stderr fifo
	Stderr() string
	// ExitFD returns the fd the provides an event when the process exits
	ExitFD() int
	// ExitStatus returns the exit status of the process or an error if it
	// has not exited
	ExitStatus() (int, error)
	Spec() specs.Process
	// Signal sends the provided signal to the process
	Signal(os.Signal) error
	// Container returns the container that the process belongs to
	Container() Container
}

func newProcess(root, id string, c *container, s specs.Process) (*process, error) {
	p := &process{
		root:      root,
		id:        id,
		container: c,
		spec:      s,
	}
	f, err := os.Create(filepath.Join(root, "process.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(s); err != nil {
		return nil, err
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
	return p, nil
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
