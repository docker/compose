package main

import (
	"path/filepath"
	"runtime"

	"github.com/docker/containerkit"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func NewBindDriver() containerkit.GraphDriver {
	return &bindDriver{}
}

// this demos how the graph/layer subsystem will create the rootfs and
// provide it to the container, the Mount type ties the execution and
// filesystem layers together
type bindDriver struct {
}

func (b *bindDriver) Mount(id string) (*containerkit.Mount, error) {
	return &containerkit.Mount{
		Target: "/",
		Type:   "bind",
		Source: "/containers/redis/rootfs",
		Options: []string{
			"rbind",
			"rw",
		},
	}, nil
}

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

func (t *testConfig) Spec(m *containerkit.Mount) (*specs.Spec, error) {
	cgpath := filepath.Join("/containerkit", t.ID())
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
			Resources: &specs.Resources{
				Devices: []specs.DeviceCgroup{
					{
						Allow:  false,
						Access: &RWM,
					},
				},
			},
			Namespaces: []specs.Namespace{
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
