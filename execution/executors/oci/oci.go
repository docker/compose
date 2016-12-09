package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/crosbymichael/go-runc"
	"github.com/docker/containerd/execution"
)

var ErrRootEmpty = errors.New("oci: runtime root cannot be an empty string")

func New(root string) *OCIRuntime {
	return &OCIRuntime{
		root: root,
		runc: &runc.Runc{
			Root: filepath.Join(root, "runc"),
		},
		ios: make(map[string]runc.IO),
	}
}

type OCIRuntime struct {
	// root holds runtime state information for the containers
	root string
	runc *runc.Runc

	// We need to keep track of the created IO for
	ios map[string]runc.IO
}

func closeRuncIO(io runc.IO) {
	if io.Stdin != nil {
		io.Stdin.(*os.File).Close()
	}
	if io.Stdout != nil {
		io.Stdout.(*os.File).Close()
	}
	if io.Stderr != nil {
		io.Stderr.(*os.File).Close()
	}
}

func getRuncIO(stdin, stdout, stderr string) (io runc.IO, err error) {
	defer func() {
		if err != nil {
			closeRuncIO(io)
		}
	}()
	if io.Stdin, err = os.OpenFile(stdin, os.O_RDONLY, 0); err != nil {
		return
	}
	if io.Stdout, err = os.OpenFile(stdout, os.O_WRONLY, 0); err != nil {
		return
	}
	if io.Stderr, err = os.OpenFile(stderr, os.O_WRONLY, 0); err != nil {
		return
	}
	return
}

func setupConsole(rio runc.IO) (*os.File, string, error) {
	master, console, err := newConsole(0, 0)
	if err != nil {
		return nil, "", err
	}
	go io.Copy(master, rio.Stdin)
	go func() {
		io.Copy(rio.Stdout, master)
		master.Close()
	}()

	return master, console, nil
}

func (r *OCIRuntime) Create(ctx context.Context, id string, o execution.CreateOpts) (container *execution.Container, err error) {
	rio, err := getRuncIO(o.Stdin, o.Stdout, o.Stderr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			closeRuncIO(rio)
		}
	}()
	consolePath := ""
	if o.Console {
		master, console, err := setupConsole(rio)
		if err != nil {
			return nil, err
		}
		consolePath = console
		defer func() {
			if err != nil {
				master.Close()
			}
		}()
	}

	if container, err = execution.NewContainer(r.root, id, o.Bundle, "created"); err != nil {
		return nil, err
	}
	defer func(c *execution.Container) {
		if err != nil {
			c.StateDir().Delete()
		}
	}(container)

	initDir, err := container.StateDir().NewProcess()
	if err != nil {
		return nil, err
	}
	pidFile := filepath.Join(initDir, "pid")
	err = r.runc.Create(ctx, id, o.Bundle, &runc.CreateOpts{
		PidFile: pidFile,
		Console: consolePath,
		IO:      rio,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			r.runc.Kill(ctx, id, int(syscall.SIGKILL))
			r.runc.Delete(ctx, id)
		}
	}()

	pid, err := runc.ReadPidFile(pidFile)
	if err != nil {
		return nil, err
	}
	process, err := newProcess(filepath.Base(initDir), pid)
	if err != nil {
		return nil, err
	}

	container.AddProcess(process, true)

	r.ios[id] = rio

	return container, nil
}

func (r *OCIRuntime) Start(ctx context.Context, c *execution.Container) error {
	return r.runc.Start(ctx, c.ID())
}

func (r *OCIRuntime) Status(ctx context.Context, c *execution.Container) (execution.Status, error) {
	state, err := r.runc.State(ctx, c.ID())
	if err != nil {
		return "", err
	}
	return execution.Status(state.Status), nil
}

func (r *OCIRuntime) load(runcC *runc.Container) (*execution.Container, error) {
	container := execution.LoadContainer(
		execution.StateDir(filepath.Join(r.root, runcC.ID)),
		runcC.ID,
		runcC.Bundle,
		runcC.Status,
		int64(runcC.Pid),
	)

	dirs, err := container.StateDir().Processes()
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		pid, err := runc.ReadPidFile(filepath.Join(d, "pid"))
		if err != nil {
			if os.IsNotExist(err) {
				// Process died in between
				continue
			}
			return nil, err
		}
		process, err := newProcess(filepath.Base(d), pid)
		if err != nil {
			return nil, err
		}
		container.AddProcess(process, pid == runcC.Pid)
	}

	return container, nil
}

func (r *OCIRuntime) List(ctx context.Context) ([]*execution.Container, error) {
	runcCs, err := r.runc.List(ctx)
	if err != nil {
		return nil, err
	}

	var containers []*execution.Container
	for _, c := range runcCs {
		container, err := r.load(c)
		if err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}

	return containers, nil
}

func (r *OCIRuntime) Load(ctx context.Context, id string) (*execution.Container, error) {
	runcC, err := r.runc.State(ctx, id)
	if err != nil {
		return nil, err
	}

	return r.load(runcC)
}

func (r *OCIRuntime) Delete(ctx context.Context, c *execution.Container) error {
	id := c.ID()
	if err := r.runc.Delete(ctx, id); err != nil {
		return err
	}
	c.StateDir().Delete()
	closeRuncIO(r.ios[id])
	delete(r.ios, id)
	return nil
}

func (r *OCIRuntime) Pause(ctx context.Context, c *execution.Container) error {
	return r.runc.Pause(ctx, c.ID())
}

func (r *OCIRuntime) Resume(ctx context.Context, c *execution.Container) error {
	return r.runc.Resume(ctx, c.ID())
}

func (r *OCIRuntime) StartProcess(ctx context.Context, c *execution.Container, o execution.StartProcessOpts) (p execution.Process, err error) {
	rio, err := getRuncIO(o.Stdin, o.Stdout, o.Stderr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			closeRuncIO(rio)
		}
	}()
	consolePath := ""
	if o.Console {
		master, console, err := setupConsole(rio)
		if err != nil {
			return nil, err
		}
		consolePath = console
		defer func() {
			if err != nil {
				master.Close()
			}
		}()
	}

	processStateDir, err := c.StateDir().NewProcess()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			c.StateDir().DeleteProcess(filepath.Base(processStateDir))
		}
	}()

	pidFile := filepath.Join(processStateDir, "pid")
	if err := r.runc.Exec(ctx, c.ID(), o.Spec, &runc.ExecOpts{
		PidFile: pidFile,
		Detach:  false,
		Console: consolePath,
		Cwd:     o.Spec.Cwd,
		IO:      rio,
	}); err != nil {
		return nil, err
	}
	pid, err := runc.ReadPidFile(pidFile)
	if err != nil {
		return nil, err
	}

	process, err := newProcess(filepath.Base(processStateDir), pid)
	if err != nil {
		return nil, err
	}

	c.AddProcess(process, false)

	r.ios[fmt.Sprintf("%s-%s", c.ID(), process.ID())] = rio

	return process, nil
}

func (r *OCIRuntime) SignalProcess(ctx context.Context, c *execution.Container, id string, sig os.Signal) error {
	process := c.GetProcess(id)
	if process == nil {
		return fmt.Errorf("Make a Process Not Found error")
	}
	return syscall.Kill(int(process.Pid()), sig.(syscall.Signal))
}

func (r *OCIRuntime) DeleteProcess(ctx context.Context, c *execution.Container, id string) error {
	ioID := fmt.Sprintf("%s-%s", c.ID(), id)
	closeRuncIO(r.ios[ioID])
	delete(r.ios, ioID)
	return c.StateDir().DeleteProcess(id)
}
