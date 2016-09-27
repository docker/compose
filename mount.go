package containerkit

// Mount is the lingua franca of the containerkit. A mount represents a
// serialized mount syscall. Components either emit or consume mounts.
type Mount struct {
	// Type specifies the host-specific of the mount.
	Type string

	// Source specifies where to mount from. Depending on the host system, this
	// can be a source path or device.
	Source string

	// Target is the filesystem mount location.
	Target string

	// Options contains zero or more fstab-style mount options. Typically,
	// these are platform specific.
	Options []string
}
