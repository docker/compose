package shim

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/docker/containerd/execution"
	"github.com/docker/containerd/log"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	DefaultShimBinary = "containerd-shim"

	pidFilename         = "pid"
	startTimeFilename   = "starttime"
	exitPipeFilename    = "exit"
	controlPipeFilename = "control"
	initProcessID       = "init"
	exitStatusFilename  = "exitStatus"
)

func New(ctx context.Context, root, shim, runtime string, runtimeArgs []string) (*ShimRuntime, error) {
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, errors.Wrap(err, "epollcreate1 failed")
	}
	s := &ShimRuntime{
		ctx:          ctx,
		epollFd:      fd,
		root:         root,
		binaryName:   shim,
		runtime:      runtime,
		runtimeArgs:  runtimeArgs,
		exitChannels: make(map[int]*process),
		containers:   make(map[string]*execution.Container),
	}

	s.loadContainers()

	go s.monitor()

	return s, nil
}

type ShimRuntime struct {
	ctx context.Context

	mutex        sync.Mutex
	exitChannels map[int]*process
	containers   map[string]*execution.Container

	epollFd     int
	root        string
	binaryName  string
	runtime     string
	runtimeArgs []string
}

type ProcessOpts struct {
	Bundle   string
	Terminal bool
	Stdin    string
	Stdout   string
	Stderr   string
}

type processState struct {
	specs.Process
	Exec           bool     `json:"exec"`
	Stdin          string   `json:"containerdStdin"`
	Stdout         string   `json:"containerdStdout"`
	Stderr         string   `json:"containerdStderr"`
	RuntimeArgs    []string `json:"runtimeArgs"`
	NoPivotRoot    bool     `json:"noPivotRoot"`
	CheckpointPath string   `json:"checkpoint"`
	RootUID        int      `json:"rootUID"`
	RootGID        int      `json:"rootGID"`
}

func (s *ShimRuntime) Create(ctx context.Context, id string, o execution.CreateOpts) (*execution.Container, error) {
	log.G(s.ctx).WithFields(logrus.Fields{"container-id": id, "options": o}).Debug("Create()")

	if s.getContainer(id) != nil {
		return nil, execution.ErrContainerExists
	}

	container, err := execution.NewContainer(s.root, id, o.Bundle)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			container.StateDir().Delete()
		}
	}()

	err = ioutil.WriteFile(filepath.Join(string(container.StateDir()), "bundle"), []byte(o.Bundle), 0600)
	if err != nil {
		return nil, errors.Wrap(err, "failed to save bundle path to disk")
	}

	// extract Process spec from bundle's config.json
	var spec specs.Spec
	f, err := os.Open(filepath.Join(o.Bundle, "config.json"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open config.json")
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		return nil, errors.Wrap(err, "failed to decode container OCI specs")
	}

	processOpts := newProcessOpts{
		shimBinary:  s.binaryName,
		runtime:     s.runtime,
		runtimeArgs: s.runtimeArgs,
		container:   container,
		exec:        false,
		StartProcessOpts: execution.StartProcessOpts{
			ID:      initProcessID,
			Spec:    spec.Process,
			Console: o.Console,
			Stdin:   o.Stdin,
			Stdout:  o.Stdout,
			Stderr:  o.Stderr,
		},
	}

	process, err := newProcess(ctx, processOpts)
	if err != nil {
		return nil, err
	}
	process.ctx = log.WithModule(log.WithModule(s.ctx, "container"), id)

	s.monitorProcess(process)
	container.AddProcess(process, true)

	s.addContainer(container)

	return container, nil
}

func (s *ShimRuntime) Start(ctx context.Context, c *execution.Container) error {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c}).Debug("Start()")

	cmd := exec.CommandContext(ctx, s.runtime, append(s.runtimeArgs, "start", c.ID())...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "'%s start' failed with output: %v", s.runtime, string(out))
	}
	return nil
}

func (s *ShimRuntime) List(ctx context.Context) ([]*execution.Container, error) {
	log.G(s.ctx).Debug("List()")

	containers := make([]*execution.Container, 0)
	s.mutex.Lock()
	for _, c := range s.containers {
		containers = append(containers, c)
	}
	s.mutex.Unlock()

	return containers, nil
}

func (s *ShimRuntime) Load(ctx context.Context, id string) (*execution.Container, error) {
	log.G(s.ctx).WithFields(logrus.Fields{"container-id": id}).Debug("Start()")

	s.mutex.Lock()
	c, ok := s.containers[id]
	s.mutex.Unlock()

	if !ok {
		return nil, errors.New(execution.ErrContainerNotFound.Error())
	}

	return c, nil
}

func (s *ShimRuntime) Delete(ctx context.Context, c *execution.Container) error {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c}).Debug("Delete()")

	if c.Status() != execution.Stopped {
		return errors.Errorf("cannot delete a container in the '%s' state", c.Status())
	}

	c.StateDir().Delete()
	s.removeContainer(c)
	return nil
}

func (s *ShimRuntime) Pause(ctx context.Context, c *execution.Container) error {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c}).Debug("Pause()")

	cmd := exec.CommandContext(ctx, s.runtime, append(s.runtimeArgs, "pause", c.ID())...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "'%s pause' failed with output: %v", s.runtime, string(out))
	}
	return nil
}

func (s *ShimRuntime) Resume(ctx context.Context, c *execution.Container) error {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c}).Debug("Resume()")

	cmd := exec.CommandContext(ctx, s.runtime, append(s.runtimeArgs, "resume", c.ID())...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "'%s resume' failed with output: %v", s.runtime, string(out))
	}
	return nil
}

func (s *ShimRuntime) StartProcess(ctx context.Context, c *execution.Container, o execution.StartProcessOpts) (p execution.Process, err error) {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c, "options": o}).Debug("StartProcess()")

	processOpts := newProcessOpts{
		shimBinary:       s.binaryName,
		runtime:          s.runtime,
		runtimeArgs:      s.runtimeArgs,
		container:        c,
		exec:             true,
		StartProcessOpts: o,
	}
	process, err := newProcess(ctx, processOpts)
	if err != nil {
		return nil, err
	}

	process.status = execution.Running
	s.monitorProcess(process)

	c.AddProcess(process, false)
	return process, nil
}

func (s *ShimRuntime) SignalProcess(ctx context.Context, c *execution.Container, id string, sig os.Signal) error {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c, "process-id": id, "signal": sig}).
		Debug("SignalProcess()")

	process := c.GetProcess(id)
	if process == nil {
		return errors.Errorf("no such process %s", id)
	}
	err := syscall.Kill(int(process.Pid()), sig.(syscall.Signal))
	if err != nil {
		return errors.Wrapf(err, "failed to send %v signal to process %v", sig, process.Pid())
	}
	return err
}

func (s *ShimRuntime) DeleteProcess(ctx context.Context, c *execution.Container, id string) error {
	log.G(s.ctx).WithFields(logrus.Fields{"container": c, "process-id": id}).
		Debug("DeleteProcess()")

	c.RemoveProcess(id)
	return c.StateDir().DeleteProcess(id)
}

//
//
//

func (s *ShimRuntime) monitor() {
	var events [128]syscall.EpollEvent
	for {
		n, err := syscall.EpollWait(s.epollFd, events[:], -1)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			log.G(s.ctx).Error("epollwait failed:", err)
		}

		for i := 0; i < n; i++ {
			fd := int(events[i].Fd)

			s.mutex.Lock()
			p := s.exitChannels[fd]
			delete(s.exitChannels, fd)
			s.mutex.Unlock()

			if err = syscall.EpollCtl(s.epollFd, syscall.EPOLL_CTL_DEL, fd, &syscall.EpollEvent{
				Events: syscall.EPOLLHUP,
				Fd:     int32(fd),
			}); err != nil {
				log.G(s.ctx).Error("epollctl deletion failed:", err)
			}

			close(p.exitChan)
		}
	}
}

func (s *ShimRuntime) addContainer(c *execution.Container) {
	s.mutex.Lock()
	s.containers[c.ID()] = c
	s.mutex.Unlock()
}

func (s *ShimRuntime) removeContainer(c *execution.Container) {
	s.mutex.Lock()
	delete(s.containers, c.ID())
	s.mutex.Unlock()
}

func (s *ShimRuntime) getContainer(id string) *execution.Container {
	s.mutex.Lock()
	c := s.containers[id]
	s.mutex.Unlock()

	return c
}

// monitorProcess adds a process to the list of monitored process if
// we fail to do so, we closed the exitChan channel used by Wait().
// Since service always call on Wait() for generating "exit" events,
// this will ensure the process gets killed
func (s *ShimRuntime) monitorProcess(p *process) {
	if p.status == execution.Stopped {
		close(p.exitChan)
		return
	}

	fd := int(p.exitPipe.Fd())
	event := syscall.EpollEvent{
		Fd:     int32(fd),
		Events: syscall.EPOLLHUP,
	}
	s.mutex.Lock()
	s.exitChannels[fd] = p
	s.mutex.Unlock()
	if err := syscall.EpollCtl(s.epollFd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		s.mutex.Lock()
		delete(s.exitChannels, fd)
		s.mutex.Unlock()
		close(p.exitChan)
		return
	}

	// TODO: take care of the OOM handler
}

func (s *ShimRuntime) unmonitorProcess(p *process) {
	s.mutex.Lock()
	for fd, proc := range s.exitChannels {
		if proc == p {
			delete(s.exitChannels, fd)
			break
		}
	}
	s.mutex.Unlock()
}

func (s *ShimRuntime) loadContainers() {
	cs, err := ioutil.ReadDir(s.root)
	if err != nil {
		log.G(s.ctx).WithField("statedir", s.root).
			Warn("failed to load containers, state dir cannot be listed:", err)
		return
	}

	for _, c := range cs {
		if !c.IsDir() {
			continue
		}

		stateDir, err := execution.LoadStateDir(s.root, c.Name())
		if err != nil {
			// We should never fail the above call unless someone
			// delete the directory while we're loading
			log.G(s.ctx).WithFields(logrus.Fields{"container": c.Name(), "statedir": s.root}).
				Warn("failed to load container statedir:", err)
			continue
		}
		bundle, err := ioutil.ReadFile(filepath.Join(string(stateDir), "bundle"))
		if err != nil {
			log.G(s.ctx).WithField("container", c.Name()).
				Warn("failed to load container bundle path:", err)
			continue
		}

		container := execution.LoadContainer(stateDir, c.Name(), string(bundle), execution.Unknown)
		s.addContainer(container)

		processDirs, err := stateDir.Processes()
		if err != nil {
			log.G(s.ctx).WithField("container", c.Name()).
				Warn("failed to retrieve container processes:", err)
			continue
		}

		for _, procStateRoot := range processDirs {
			id := filepath.Base(procStateRoot)
			proc, err := loadProcess(procStateRoot, id)
			if err != nil {
				log.G(s.ctx).WithFields(logrus.Fields{"container": c.Name(), "process": id}).
					Warn("failed to load process:", err)
				s.removeContainer(container)
				for _, p := range container.Processes() {
					s.unmonitorProcess(p.(*process))
				}
				break
			}
			proc.ctx = log.WithModule(log.WithModule(s.ctx, "container"), container.ID())
			container.AddProcess(proc, proc.ID() == initProcessID)
			s.monitorProcess(proc)
		}
	}
}
