package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs"
)

type process struct {
	id           string
	bundle       string
	stdio        *stdio
	s            specs.Process
	exec         bool
	containerPid int
}

func newProcess(id, bundle string, exec bool) (*process, error) {
	f, err := os.Open("process.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	p := &process{
		id:     id,
		bundle: bundle,
		exec:   exec,
	}
	if err := json.NewDecoder(f).Decode(&p.s); err != nil {
		return nil, err
	}
	if err := p.openIO(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *process) start() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	args := []string{
		"--id", p.id,
	}
	if p.exec {
		args = append(args, "exec",
			"--process", filepath.Join(cwd, "process.json"))
	} else {
		args = append(args, "start",
			"--bundle", p.bundle)
	}
	args = append(args,
		"-d",
		"--console", p.stdio.console,
		"--pid-file", filepath.Join(cwd, "pid"),
	)
	cmd := exec.Command("runc", args...)
	cmd.Stdin = p.stdio.stdin
	cmd.Stdout = p.stdio.stdout
	cmd.Stderr = p.stdio.stderr
	// set the parent death signal to SIGKILL so that if the shim dies the container
	// process also dies
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	data, err := ioutil.ReadFile("pid")
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return err
	}
	p.containerPid = pid
	return nil

}

func (p *process) pid() int {
	return p.containerPid
}

func (p *process) delete() error {
	if !p.exec {
		return exec.Command("runc", "--id", p.id, "delete").Run()
	}
	return nil
}

// openIO opens the pre-created fifo's for use with the container
// in RDWR so that they remain open if the other side stops listening
func (p *process) openIO() error {
	p.stdio = &stdio{}
	if p.s.Terminal {
		// FIXME: this is wrong for user namespaces and will need to be translated
		console, err := libcontainer.NewConsole(int(p.s.User.UID), int(p.s.User.GID))
		if err != nil {
			return err
		}
		p.stdio.console = console.Path()
		stdin, err := os.OpenFile("stdin", syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		go func() {
			io.Copy(console, stdin)
		}()
		stdout, err := os.OpenFile("stdout", syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		go func() {
			io.Copy(stdout, console)
			console.Close()
		}()
		return nil
	}
	// non-tty
	for name, dest := range map[string]**os.File{
		"stdin":  &p.stdio.stdin,
		"stdout": &p.stdio.stdout,
		"stderr": &p.stdio.stderr,
	} {
		f, err := os.OpenFile(name, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		*dest = f
	}
	return nil
}

func (p *process) Close() error {
	return p.stdio.Close()
}

type stdio struct {
	stdin   *os.File
	stdout  *os.File
	stderr  *os.File
	console string
}

func (s *stdio) Close() error {
	err := s.stdin.Close()
	if oerr := s.stdout.Close(); err == nil {
		err = oerr
	}
	if oerr := s.stderr.Close(); err == nil {
		err = oerr
	}
	return err
}
