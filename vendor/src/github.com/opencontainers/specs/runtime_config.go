package specs

// RuntimeSpec contains host-specific configuration information for
// a container. This information must not be included when the bundle
// is packaged for distribution.
type RuntimeSpec struct {
	// Mounts is a mapping of names to mount configurations.
	// Which mounts will be mounted and where should be chosen with MountPoints
	// in Spec.
	Mounts map[string]Mount `json:"mounts"`
	// Hooks are the commands run at various lifecycle events of the container.
	Hooks Hooks `json:"hooks"`
}

// Hook specifies a command that is run at a particular event in the lifecycle of a container
type Hook struct {
	Path string   `json:"path"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
}

// Hooks for container setup and teardown
type Hooks struct {
	// Prestart is a list of hooks to be run before the container process is executed.
	// On Linux, they are run after the container namespaces are created.
	Prestart []Hook `json:"prestart,omitempty"`
	// Poststart is a list of hooks to be run after the container process is started.
	Poststart []Hook `json:"poststart,omitempty"`
	// Poststop is a list of hooks to be run after the container process exits.
	Poststop []Hook `json:"poststop,omitempty"`
}

// Mount specifies a mount for a container
type Mount struct {
	// Type specifies the mount kind.
	Type string `json:"type"`
	// Source specifies the source path of the mount.  In the case of bind mounts on
	// linux based systems this would be the file on the host.
	Source string `json:"source"`
	// Options are fstab style mount options.
	Options []string `json:"options,omitempty"`
}
