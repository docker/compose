package specification

import (
	"runtime"

	"github.com/docker/containerd"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var rwm = "rwm"

func Default(config containerd.Config, mounts []containerd.Mount) *specs.Spec {
	s := &specs.Spec{
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
			Args:            config.Process.Args,
			Env:             config.Process.Env,
			Terminal:        config.Process.TTY,
			Cwd:             config.Process.Cwd,
			NoNewPrivileges: true,
		},
		Hostname: config.Hostname,
		Linux: &specs.Linux{
			Resources: &specs.LinuxResources{
				Devices: []specs.LinuxDeviceCgroup{
					{
						Allow:  false,
						Access: &rwm,
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
				{
					Type: "network",
				},
			},
		},
		Annotations: config.Labels,
	}
	// apply snapshot mounts
	for _, m := range mounts {
		s.Mounts = append(s.Mounts, specs.Mount{
			Source:      m.Source,
			Destination: "/",
			Type:        m.Type,
			Options:     m.Options,
		})
	}
	return s
}
