package shim

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/docker/containerkit"
	"github.com/docker/containerkit/monitor"
	"github.com/docker/containerkit/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

/*
├── libcontainerd
│   ├── containerd
│   │   └── ff2e86955c2be43f0e3c300fbd3786599301bd8efcaa5a386587f132e73af242
│   │       ├── init
│   │       │   ├── control
│   │       │   ├── exit
│   │       │   ├── log.json
│   │       │   ├── pid
│   │       │   ├── process.json
│   │       │   ├── shim-log.json
│   │       │   └── starttime
│   │       └── state.json
*/

var (
	ErrNotFifo             = errors.New("shim: IO is not a valid fifo on disk")
	errInitProcessNotExist = errors.New("shim: init process does not exist")
)

type Opts struct {
	Name        string
	RuntimeName string
	RuntimeArgs []string
	RuntimeRoot string
	NoPivotRoot bool
	Root        string
	Timeout     time.Duration
}

func New(opts Opts) (*Shim, error) {
	if err := os.MkdirAll(filepath.Dir(opts.Root), 0711); err != nil {
		return nil, err
	}
	if err := os.Mkdir(opts.Root, 0711); err != nil {
		return nil, err
	}
	r, err := oci.New(oci.Opts{
		Root: opts.RuntimeRoot,
		Name: opts.RuntimeName,
		Args: opts.RuntimeArgs,
	})
	if err != nil {
		return nil, err
	}
	m, err := monitor.New()
	if err != nil {
		return nil, err
	}
	s := &Shim{
		root:      opts.Root,
		name:      opts.Name,
		timeout:   opts.Timeout,
		runtime:   r,
		processes: make(map[string]*process),
		m:         m,
	}
	go s.startMonitor()
	f, err := os.Create(filepath.Join(opts.Root, "state.json"))
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(f).Encode(s)
	f.Close()
	return s, err
}

// Load will load an existing shim with all its information restored from the
// provided path
func Load(root string) (*Shim, error) {
	f, err := os.Open(filepath.Join(root, "state.json"))
	if err != nil {
		return nil, err
	}
	var s Shim
	err = json.NewDecoder(f).Decode(&s)
	f.Close()
	if err != nil {
		return nil, err
	}
	m, err := monitor.New()
	if err != nil {
		return nil, err
	}
	s.m = m
	go s.startMonitor()
	dirs, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		if f, err = os.Open(filepath.Join(root, name, "process.json")); err != nil {
			return nil, err
		}
		var p process
		err = json.NewDecoder(f).Decode(&p)
		f.Close()
		if err != nil {
			return nil, err
		}
		s.processes[name] = &p
		if err := s.m.Add(&p); err != nil {
			return nil, err
		}
	}
	return &s, nil
}

// Shim is a container runtime that adds a shim process as the container's parent
// to hold open stdio and other resources so that higher level daemons can exit and
// load running containers for handling upgrades and/or crashes
//
// The shim uses an OCI compliant runtime as its executor
type Shim struct {
	// root holds runtime state information for the containers
	// launched by the runtime
	root        string
	name        string
	timeout     time.Duration
	noPivotRoot bool
	runtime     *oci.OCIRuntime
	pmu         sync.Mutex
	processes   map[string]*process
	bundle      string
	checkpoint  string
	m           *monitor.Monitor
}

type state struct {
	Root string `json:"root"`
	// Bundle is the path to the container's bundle
	Bundle string `json:"bundle"`
	// OCI runtime binary name
	Runtime string `json:"runtime"`
	// OCI runtime args
	RuntimeArgs []string `json:"runtimeArgs"`
	RuntimeRoot string   `json:"runtimeRoot"`
	// Shim binary name
	Name string `json:"shim"`
	/// NoPivotRoot option
	NoPivotRoot bool `json:"noPivotRoot"`
	// Timeout for container start
	Timeout time.Duration `json:"timeout"`
}

func (s *Shim) MarshalJSON() ([]byte, error) {
	st := state{
		Name:        s.name,
		Bundle:      s.bundle,
		Runtime:     s.runtime.Name(),
		RuntimeArgs: s.runtime.Args(),
		RuntimeRoot: s.runtime.Root(),
		NoPivotRoot: s.noPivotRoot,
		Timeout:     s.timeout,
		Root:        s.root,
	}
	return json.Marshal(st)
}

func (s *Shim) UnmarshalJSON(b []byte) error {
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	s.root = st.Root
	s.name = st.Name
	s.bundle = st.Bundle
	s.timeout = st.Timeout
	s.noPivotRoot = st.NoPivotRoot
	r, err := oci.New(oci.Opts{
		Name: st.Runtime,
		Args: st.RuntimeArgs,
		Root: st.RuntimeRoot,
	})
	if err != nil {
		return err
	}
	s.runtime = r
	s.processes = make(map[string]*process)
	return nil
}

func (s *Shim) Create(c *containerkit.Container) (containerkit.ProcessDelegate, error) {
	s.bundle = c.Path()
	var (
		root = filepath.Join(s.root, "init")
		cmd  = s.command(c.ID(), c.Path(), s.runtime.Name())
	)
	if err := os.Mkdir(root, 0711); err != nil {
		return nil, err
	}
	// exec the shim inside the state directory setup with the process
	// information for what is being run
	cmd.Dir = root
	// make sure the shim is in a new process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	p, err := s.startCommand(processOpts{
		spec:        c.Spec().Process,
		root:        root,
		noPivotRoot: s.noPivotRoot,
		checkpoint:  s.checkpoint,
		c:           c,
		cmd:         cmd,
		stdin:       c.Stdin,
		stdout:      c.Stdout,
		stderr:      c.Stderr,
	})
	if err != nil {
		return nil, err
	}
	s.pmu.Lock()
	s.processes["init"] = p
	s.pmu.Unlock()
	f, err := os.Create(filepath.Join(s.root, "state.json"))
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(f).Encode(s)
	f.Close()
	// ~TODO: oom and stats stuff here
	return p, err
}

func (s *Shim) Start(c *containerkit.Container) error {
	p, err := s.getContainerInit()
	if err != nil {
		return err
	}
	var (
		errC = make(chan error, 1)
		cmd  = s.runtime.Command("start", c.ID())
	)
	go func() {
		out, err := cmd.CombinedOutput()
		if err != nil {
			errC <- fmt.Errorf("%s: %q", err, out)
		}
		errC <- nil
	}()
	select {
	case err := <-errC:
		if err != nil {
			return err
		}
	case <-p.done:
		if !p.success {
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
	return nil
}

func (s *Shim) Delete(c *containerkit.Container) error {
	if err := s.runtime.Delete(c); err != nil {
		return err
	}
	return os.RemoveAll(s.root)
}

func (s *Shim) Exec(c *containerkit.Container, p *containerkit.Process) (containerkit.ProcessDelegate, error) {
	root, err := ioutil.TempDir(s.root, "")
	if err != nil {
		return nil, err
	}
	cmd := s.command(c.ID(), c.Path(), s.runtime.Name())
	// exec the shim inside the state directory setup with the process
	// information for what is being run
	cmd.Dir = root
	// make sure the shim is in a new process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	sp, err := s.startCommand(processOpts{
		exec:        true,
		spec:        *p.Spec(),
		root:        root,
		noPivotRoot: s.noPivotRoot,
		checkpoint:  s.checkpoint,
		c:           c,
		cmd:         cmd,
		stdin:       p.Stdin,
		stdout:      p.Stdout,
		stderr:      p.Stderr,
	})
	if err != nil {
		return nil, err
	}
	s.pmu.Lock()
	s.processes[filepath.Base(root)] = sp
	s.pmu.Unlock()
	return sp, nil
}

func (s *Shim) Load(id string) (containerkit.ProcessDelegate, error) {
	return s.getContainerInit()
}

func (s *Shim) getContainerInit() (*process, error) {
	s.pmu.Lock()
	p, ok := s.processes["init"]
	s.pmu.Unlock()
	if !ok {
		return nil, errInitProcessNotExist
	}
	return p, nil
}

func (s *Shim) startCommand(opts processOpts) (*process, error) {
	p, err := newProcess(opts)
	if err != nil {
		return nil, err
	}
	if err := s.m.Add(p); err != nil {
		return nil, err
	}
	if err := opts.cmd.Start(); err != nil {
		close(p.done)
		if checkShimNotFound(err) {
			return nil, fmt.Errorf("%s not install on system", s.name)
		}
		return nil, err
	}
	// make sure it does not die before we get the container's pid
	defer func() {
		go p.checkExited()
	}()
	if err := p.waitForCreate(s.timeout); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Shim) command(args ...string) *exec.Cmd {
	return exec.Command(s.name, args...)
}

func (s *Shim) startMonitor() {
	go s.m.Run()
	defer s.m.Close()
	for m := range s.m.Events() {
		p := m.(*process)
		close(p.done)
	}
}

// checkShimNotFound checks the error returned from a exec call to see if the binary
// that was called exists on the system and returns true if the shim binary does not exist
func checkShimNotFound(err error) bool {
	if exitError, ok := err.(*exec.Error); ok {
		e := exitError.Err
		return e == exec.ErrNotFound || e == os.ErrNotExist
	}
	return false
}

// getFifoPath returns the path to the fifo on disk as long as the provided
// interface is an *os.File and has a valid path on the Name() method call
func getFifoPath(v interface{}) (string, error) {
	f, ok := v.(*os.File)
	if !ok {
		return "", ErrNotFifo
	}
	p := f.Name()
	if p == "" {
		return "", ErrNotFifo
	}
	return p, nil
}

func getRootIDs(s *specs.Spec) (int, int, error) {
	if s == nil {
		return 0, 0, nil
	}
	var hasUserns bool
	for _, ns := range s.Linux.Namespaces {
		if ns.Type == specs.UserNamespace {
			hasUserns = true
			break
		}
	}
	if !hasUserns {
		return 0, 0, nil
	}
	uid := hostIDFromMap(0, s.Linux.UIDMappings)
	gid := hostIDFromMap(0, s.Linux.GIDMappings)
	return uid, gid, nil
}

func hostIDFromMap(id uint32, mp []specs.LinuxIDMapping) int {
	for _, m := range mp {
		if (id >= m.ContainerID) && (id <= (m.ContainerID + m.Size - 1)) {
			return int(m.HostID + (id - m.ContainerID))
		}
	}
	return 0
}
