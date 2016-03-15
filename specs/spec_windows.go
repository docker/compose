package specs

// Temporary Windows version of the spec in lieu of opencontainers/specs/specs-go having
// Windows support currently.

type (
	PlatformSpec WindowsSpec
	ProcessSpec  Process
)

// This is a temporary module in lieu of opencontainers/specs/specs-go being compatible
// currently on Windows.

// Process contains information to start a specific application inside the container.
type Process struct {
	// Terminal creates an interactive terminal for the container.
	Terminal bool `json:"terminal"`
	// User specifies user information for the process.
	// TEMPORARY HACK User User `json:"user"`
	// Args specifies the binary and arguments for the application to execute.
	Args []string `json:"args"`
	// Env populates the process environment for the process.
	Env []string `json:"env,omitempty"`
	// Cwd is the current working directory for the process and must be
	// relative to the container's root.
	Cwd string `json:"cwd"`
}

type Spec struct {
	// Version is the version of the specification that is supported.
	Version string `json:"ociVersion"`
	// Platform is the host information for OS and Arch.
	// TEMPORARY HACK Platform Platform `json:"platform"`
	// Process is the container's main process.
	Process Process `json:"process"`
	// Root is the root information for the container's filesystem.
	// TEMPORARY HACK Root Root `json:"root"`
	// Hostname is the container's host name.
	// TEMPORARY HACK Hostname string `json:"hostname,omitempty"`
	// Mounts profile configuration for adding mounts to the container's filesystem.
	// TEMPORARY HACK Mounts []Mount `json:"mounts"`
	// Hooks are the commands run at various lifecycle events of the container.
	// TEMPORARY HACK Hooks Hooks `json:"hooks"`
}

// Windows contains platform specific configuration for Windows based containers.
type Windows struct {
}

// TODO Windows - Interim hack. Needs implementing.
type WindowsSpec struct {
	Spec

	// Windows is platform specific configuration for Windows based containers.
	Windows Windows `json:"windows"`
}
