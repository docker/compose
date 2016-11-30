package containerd

import (
	"os"
	"os/exec"
	"strings"
)

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

// MountCommand converts the provided mount into a CLI arguments that can be used to mount the
func MountCommand(m Mount) []string {
	return []string{
		"mount",
		"-t", strings.ToLower(m.Type),
		m.Source,
		m.Target,
		"-o", strings.Join(m.Options, ","),
	}
}

func MountAll(mounts ...Mount) error {
	for _, mount := range mounts {
		cmd := exec.Command("mount", MountCommand(mount)[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout

		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}
