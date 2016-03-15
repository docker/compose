// +build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/specs/specs-go"
)

const wildcard = -1

var errEmptyID = errors.New("container id cannot be empty")

var allowedDevices = []*configs.Device{
	// allow mknod for any device
	{
		Type:        'c',
		Major:       wildcard,
		Minor:       wildcard,
		Permissions: "m",
		Allow:       true,
	},
	{
		Type:        'b',
		Major:       wildcard,
		Minor:       wildcard,
		Permissions: "m",
		Allow:       true,
	},
	{
		Type:        'c',
		Path:        "/dev/null",
		Major:       1,
		Minor:       3,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Type:        'c',
		Path:        "/dev/random",
		Major:       1,
		Minor:       8,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Type:        'c',
		Path:        "/dev/full",
		Major:       1,
		Minor:       7,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Type:        'c',
		Path:        "/dev/tty",
		Major:       5,
		Minor:       0,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Type:        'c',
		Path:        "/dev/zero",
		Major:       1,
		Minor:       5,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Type:        'c',
		Path:        "/dev/urandom",
		Major:       1,
		Minor:       9,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Path:        "/dev/console",
		Type:        'c',
		Major:       5,
		Minor:       1,
		Permissions: "rwm",
		Allow:       true,
	},
	// /dev/pts/ - pts namespaces are "coming soon"
	{
		Path:        "",
		Type:        'c',
		Major:       136,
		Minor:       wildcard,
		Permissions: "rwm",
		Allow:       true,
	},
	{
		Path:        "",
		Type:        'c',
		Major:       5,
		Minor:       2,
		Permissions: "rwm",
		Allow:       true,
	},
	// tuntap
	{
		Path:        "",
		Type:        'c',
		Major:       10,
		Minor:       200,
		Permissions: "rwm",
		Allow:       true,
	},
}

var (
	maskedPaths = []string{
		"/proc/kcore",
		"/proc/latency_stats",
		"/proc/timer_stats",
		"/proc/sched_debug",
	}
	readonlyPaths = []string{
		"/proc/asound",
		"/proc/bus",
		"/proc/fs",
		"/proc/irq",
		"/proc/sys",
		"/proc/sysrq-trigger",
	}
)

var container libcontainer.Container

func containerPreload(context *cli.Context) error {
	c, err := getContainer(context)
	if err != nil {
		return err
	}
	container = c
	return nil
}

// loadFactory returns the configured factory instance for execing containers.
func loadFactory(context *cli.Context) (libcontainer.Factory, error) {
	var (
		debug = "false"
		root  = context.GlobalString("root")
	)
	if context.GlobalBool("debug") {
		debug = "true"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var logAbs string
	if l := context.GlobalString("log"); l != "" {
		if logAbs, err = filepath.Abs(context.GlobalString("log")); err != nil {
			return nil, err
		}
	}
	return libcontainer.New(abs, libcontainer.Cgroupfs, func(l *libcontainer.LinuxFactory) error {
		l.CriuPath = context.GlobalString("criu")
		return nil
	},
		libcontainer.InitArgs(os.Args[0],
			"--log", logAbs,
			"--log-format", context.GlobalString("log-format"),
			fmt.Sprintf("--debug=%s", debug),
			"init"),
	)
}

// getContainer returns the specified container instance by loading it from state
// with the default factory.
func getContainer(context *cli.Context) (libcontainer.Container, error) {
	id := context.Args().First()
	if id == "" {
		return nil, errEmptyID
	}
	factory, err := loadFactory(context)
	if err != nil {
		return nil, err
	}
	return factory.Load(id)
}

// fatal prints the error's details if it is a libcontainer specific error type
// then exits the program with an exit status of 1.
func fatal(err error) {
	// make sure the error is written to the logger
	logrus.Error(err)
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func fatalf(t string, v ...interface{}) {
	fatal(fmt.Errorf(t, v...))
}

func getDefaultImagePath(context *cli.Context) string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(cwd, "checkpoint")
}

// newProcess returns a new libcontainer Process with the arguments from the
// spec and stdio from the current process.
func newProcess(p specs.Process) (*libcontainer.Process, error) {
	lp := &libcontainer.Process{
		Args: p.Args,
		Env:  p.Env,
		// TODO: fix libcontainer's API to better support uid/gid in a typesafe way.
		User:            fmt.Sprintf("%d:%d", p.User.UID, p.User.GID),
		Cwd:             p.Cwd,
		Capabilities:    p.Capabilities,
		Label:           p.SelinuxLabel,
		NoNewPrivileges: &p.NoNewPrivileges,
		AppArmorProfile: p.ApparmorProfile,
	}
	for _, rlimit := range p.Rlimits {
		rl, err := createLibContainerRlimit(rlimit)
		if err != nil {
			return nil, err
		}
		lp.Rlimits = append(lp.Rlimits, rl)
	}
	return lp, nil
}

func dupStdio(process *libcontainer.Process, rootuid int) error {
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	for _, fd := range []uintptr{
		os.Stdin.Fd(),
		os.Stdout.Fd(),
		os.Stderr.Fd(),
	} {
		if err := syscall.Fchown(int(fd), rootuid, rootuid); err != nil {
			return err
		}
	}
	return nil
}

// If systemd is supporting sd_notify protocol, this function will add support
// for sd_notify protocol from within the container.
func setupSdNotify(spec *specs.Spec, notifySocket string) {
	spec.Mounts = append(spec.Mounts, specs.Mount{Destination: notifySocket, Type: "bind", Source: notifySocket, Options: []string{"bind"}})
	spec.Process.Env = append(spec.Process.Env, fmt.Sprintf("NOTIFY_SOCKET=%s", notifySocket))
}

func destroy(container libcontainer.Container) {
	if err := container.Destroy(); err != nil {
		logrus.Error(err)
	}
}

// setupIO sets the proper IO on the process depending on the configuration
// If there is a nil error then there must be a non nil tty returned
func setupIO(process *libcontainer.Process, rootuid int, console string, createTTY, detach bool) (*tty, error) {
	// detach and createTty will not work unless a console path is passed
	// so error out here before changing any terminal settings
	if createTTY && detach && console == "" {
		return nil, fmt.Errorf("cannot allocate tty if runc will detach")
	}
	if createTTY {
		return createTty(process, rootuid, console)
	}
	if detach {
		if err := dupStdio(process, rootuid); err != nil {
			return nil, err
		}
		return &tty{}, nil
	}
	return createStdioPipes(process, rootuid)
}

func createPidFile(path string, process *libcontainer.Process) error {
	pid, err := process.Pid()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d", pid)
	return err
}

func createContainer(context *cli.Context, id string, spec *specs.Spec) (libcontainer.Container, error) {
	config, err := createLibcontainerConfig(id, spec)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(config.Rootfs); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("rootfs (%q) does not exist", config.Rootfs)
		}
		return nil, err
	}

	factory, err := loadFactory(context)
	if err != nil {
		return nil, err
	}
	return factory.Create(id, config)
}

// runProcess will create a new process in the specified container
// by executing the process specified in the 'config'.
func runProcess(container libcontainer.Container, config *specs.Process, listenFDs []*os.File, console string, pidFile string, detach bool) (int, error) {
	process, err := newProcess(*config)
	if err != nil {
		return -1, err
	}

	// Add extra file descriptors if needed
	if len(listenFDs) > 0 {
		process.Env = append(process.Env, fmt.Sprintf("LISTEN_FDS=%d", len(listenFDs)), "LISTEN_PID=1")
		process.ExtraFiles = append(process.ExtraFiles, listenFDs...)
	}

	rootuid, err := container.Config().HostUID()
	if err != nil {
		return -1, err
	}

	tty, err := setupIO(process, rootuid, console, config.Terminal, detach)
	if err != nil {
		return -1, err
	}
	defer tty.Close()
	handler := newSignalHandler(tty)
	defer handler.Close()
	if err := container.Start(process); err != nil {
		return -1, err
	}
	if err := tty.ClosePostStart(); err != nil {
		return -1, err
	}
	if pidFile != "" {
		if err := createPidFile(pidFile, process); err != nil {
			process.Signal(syscall.SIGKILL)
			process.Wait()
			return -1, err
		}
	}
	if detach {
		return 0, nil
	}
	return handler.forward(process)
}

func validateProcessSpec(spec *specs.Process) error {
	if spec.Cwd == "" {
		return fmt.Errorf("Cwd property must not be empty")
	}
	if !filepath.IsAbs(spec.Cwd) {
		return fmt.Errorf("Cwd must be an absolute path")
	}
	if len(spec.Args) == 0 {
		return fmt.Errorf("args must not be empty")
	}
	return nil
}
