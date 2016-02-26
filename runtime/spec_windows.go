package runtime

type Spec struct {
	// Version is the version of the specification that is supported.
	Version string `json:"ociVersion"`
	// Platform is the host information for OS and Arch.
	// TEMPORARY HACK Platform Platform `json:"platform"`
	// Process is the container's main process.
	// TEMPORARY HACK Process Process `json:"process"`
	// Root is the root information for the container's filesystem.
	// TEMPORARY HACK Root Root `json:"root"`
	// Hostname is the container's host name.
	// TEMPORARY HACK Hostname string `json:"hostname,omitempty"`
	// Mounts profile configuration for adding mounts to the container's filesystem.
	// TEMPORARY HACK Mounts []Mount `json:"mounts"`
	// Hooks are the commands run at various lifecycle events of the container.
	// TEMPORARY HACK Hooks Hooks `json:"hooks"`
}

// TODO Windows - Interim hack. Needs implementing.
type WindowsSpec struct {
	Spec

	// Windows is platform specific configuration for Windows based containers.
	Windows Windows `json:"windows"`
}

// Windows contains platform specific configuration for Windows based containers.
type Windows struct {
}

type platformSpec WindowsSpec
