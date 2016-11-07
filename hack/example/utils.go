package main

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/docker/containerd/shim"
	"github.com/docker/containerkit"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var (
	RWM  = "rwm"
	caps = []string{
		"CAP_AUDIT_WRITE",
		"CAP_KILL",
		"CAP_FOWNER",
		"CAP_CHOWN",
		"CAP_MKNOD",
		"CAP_FSETID",
		"CAP_DAC_OVERRIDE",
		"CAP_SETFCAP",
		"CAP_SETPCAP",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_NET_BIND_SERVICE",
	}
	env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
)

type testConfig struct {
}

func (t *testConfig) ID() string {
	return "test"
}

func (t *testConfig) Root() string {
	return "/var/lib/containerkit"
}

func (t *testConfig) Runtime() (containerkit.Runtime, error) {
	return shim.New(shim.Opts{
		Root:        "/run/cshim/test",
		Name:        "containerd-shim",
		RuntimeName: "runc",
		RuntimeRoot: "/run/runc",
		Timeout:     5 * time.Second,
	})
	// TODO: support loading of runtime
	// create a new runtime runtime that implements the ExecutionDriver interface
	return shim.Load("/run/cshim/test")
}

func (t *testConfig) Spec() (*specs.Spec, error) {
	var (
		cgpath = filepath.Join("/containerkit", t.ID())
		m      = &containerkit.Mount{
			Target: "/",
			Type:   "bind",
			Source: "/containers/redis/rootfs",
			Options: []string{
				"rbind",
				"rw",
			},
		}
	)
	return &specs.Spec{
		Version: specs.Version,
		Platform: specs.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Root: specs.Root{
			Path:     "rootfs",
			Readonly: false,
		},
		Process: specs.Process{
			Env:             env,
			Args:            []string{"sleep", "30"},
			Terminal:        false,
			Cwd:             "/",
			NoNewPrivileges: true,
			Capabilities:    caps,
		},
		Hostname: "containerkit",
		Mounts: []specs.Mount{
			{
				Destination: m.Target,
				Type:        m.Type,
				Source:      m.Source,
				Options:     m.Options,
			},
			{
				Destination: "/proc",
				Type:        "proc",
				Source:      "proc",
			},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/dev/pts",
				Type:        "devpts",
				Source:      "devpts",
				Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
			},
			{
				Destination: "/dev/shm",
				Type:        "tmpfs",
				Source:      "shm",
				Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
			},
			{
				Destination: "/dev/mqueue",
				Type:        "mqueue",
				Source:      "mqueue",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/run",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      "/etc/resolv.conf",
				Options:     []string{"rbind", "ro"},
			},
			{
				Destination: "/etc/hosts",
				Type:        "bind",
				Source:      "/etc/hosts",
				Options:     []string{"rbind", "ro"},
			},
			{
				Destination: "/etc/localtime",
				Type:        "bind",
				Source:      "/etc/localtime",
				Options:     []string{"rbind", "ro"},
			},
		},
		Linux: &specs.Linux{
			CgroupsPath: &cgpath,
			Resources: &specs.LinuxResources{
				Devices: []specs.LinuxDeviceCgroup{
					{
						Allow:  false,
						Access: &RWM,
					},
				},
			},
			Namespaces: []specs.LinuxNamespace{
				{
					Type: "pid",
				},
				{
					Type: "ipc",
				},
				{
					Type: "uts",
				},
				{
					Type: "mount",
				},
			},
		},
	}, nil
}

func Stdin(n string) *os.File {
	abs, err := filepath.Abs("stdin" + n)
	if err != nil {
		panic(err)
	}
	if err := unix.Mkfifo(abs, 0755); err != nil && !os.IsExist(err) {
		panic(err)
	}
	f, err := os.OpenFile(abs, syscall.O_RDWR, 0)
	if err != nil {
		panic(err)
	}
	return f
}

func Stdout(n string) *os.File {
	abs, err := filepath.Abs("stdout" + n)
	if err != nil {
		panic(err)
	}
	if err := unix.Mkfifo(abs, 0755); err != nil && !os.IsExist(err) {
		panic(err)
	}
	f, err := os.OpenFile(abs, syscall.O_RDWR, 0)
	if err != nil {
		panic(err)
	}
	return f
}

func Stderr(n string) *os.File {
	abs, err := filepath.Abs("stderr" + n)
	if err != nil {
		panic(err)
	}
	if err := unix.Mkfifo(abs, 0755); err != nil && !os.IsExist(err) {
		panic(err)
	}
	f, err := os.OpenFile(abs, syscall.O_RDWR, 0)
	if err != nil {
		panic(err)
	}
	return f
}
