package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/util"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs"
)

const (
	bufferSize = 2048
)

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

// containerd-shim is a small shim that sits in front of a runc implementation
// that allows it to be repartented to init and handle reattach from the caller.
//
// the cwd of the shim should be the bundle for the container.  Arg1 should be the path
// to the state directory where the shim can locate fifos and other information.
//
//   └── shim
//    ├── control
//    ├── stderr
//    ├── stdin
//    ├── stdout
//    ├── pid
//    └── exit
func main() {
	if len(os.Args) < 2 {
		logrus.Fatal("shim: no arguments provided")
	}
	// start handling signals as soon as possible so that things are properly reaped
	// or if runc exits before we hit the handler
	signals := make(chan os.Signal, bufferSize)
	signal.Notify(signals)
	// set the shim as the subreaper for all orphaned processes created by the container
	if err := util.SetSubreaper(1); err != nil {
		logrus.WithField("error", err).Fatal("shim: set as subreaper")
	}
	// open the exit pipe
	f, err := os.OpenFile(filepath.Join(os.Args[1], "exit"), syscall.O_WRONLY, 0)
	if err != nil {
		logrus.WithField("error", err).Fatal("shim: open exit pipe")
	}
	defer f.Close()
	// open the fifos for use with the command
	std, err := openContainerSTDIO(os.Args[1])
	if err != nil {
		logrus.WithField("error", err).Fatal("shim: open container STDIO from fifo")
	}
	// star the container process by invoking runc
	runcPid, err := startRunc(std, os.Args[2])
	if err != nil {
		logrus.WithField("error", err).Fatal("shim: start runc")
	}
	var exitShim bool
	for s := range signals {
		logrus.WithField("signal", s).Debug("shim: received signal")
		switch s {
		case syscall.SIGCHLD:
			exits, err := util.Reap()
			if err != nil {
				logrus.WithField("error", err).Error("shim: reaping child processes")
			}
			for _, e := range exits {
				// check to see if runc is one of the processes that has exited
				if e.Pid == runcPid {
					exitShim = true
					logrus.WithFields(logrus.Fields{"pid": e.Pid, "status": e.Status}).Info("shim: runc exited")

					if err := writeInt(filepath.Join(os.Args[1], "exitStatus"), e.Status); err != nil {
						logrus.WithFields(logrus.Fields{"error": err, "status": e.Status}).Error("shim: write exit status")
					}
				}
			}
		}
		// runc has exited so the shim can also exit
		if exitShim {
			if err := std.Close(); err != nil {
				logrus.WithField("error", err).Error("shim: close stdio")
			}
			if err := deleteContainer(os.Args[2]); err != nil {
				logrus.WithField("error", err).Error("shim: delete runc state")
			}
			return
		}
	}
}

// startRunc starts runc detached and returns the container's pid
func startRunc(s *stdio, id string) (int, error) {
	pidFile := filepath.Join(os.Args[1], "pid")
	cmd := exec.Command("runc", "--id", id, "start", "-d", "--console", s.console, "--pid-file", pidFile)
	cmd.Stdin = s.stdin
	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr
	// set the parent death signal to SIGKILL so that if the shim dies the container
	// process also dies
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	if err := cmd.Run(); err != nil {
		return -1, err
	}
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(string(data))
}

func deleteContainer(id string) error {
	return exec.Command("runc", "--id", id, "delete").Run()
}

// openContainerSTDIO opens the pre-created fifo's for use with the container
// in RDWR so that they remain open if the other side stops listening
func openContainerSTDIO(dir string) (*stdio, error) {
	s := &stdio{}
	spec, err := getSpec()
	if err != nil {
		return nil, err
	}
	if spec.Process.Terminal {
		console, err := libcontainer.NewConsole(int(spec.Process.User.UID), int(spec.Process.User.GID))
		if err != nil {
			return nil, err
		}
		s.console = console.Path()
		stdin, err := os.OpenFile(filepath.Join(dir, "stdin"), syscall.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		go func() {
			io.Copy(console, stdin)
		}()
		stdout, err := os.OpenFile(filepath.Join(dir, "stdout"), syscall.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		go func() {
			io.Copy(stdout, console)
			console.Close()
		}()
		return s, nil
	}
	for name, dest := range map[string]**os.File{
		"stdin":  &s.stdin,
		"stdout": &s.stdout,
		"stderr": &s.stderr,
	} {
		f, err := os.OpenFile(filepath.Join(dir, name), syscall.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		*dest = f
	}
	return s, nil
}

func getSpec() (*specs.Spec, error) {
	var s specs.Spec
	f, err := os.Open("config.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func writeInt(path string, i int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d", i)
	return err
}
