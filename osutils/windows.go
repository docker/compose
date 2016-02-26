// +build windows

package osutils

// GetOpenFds returns the number of open fds for the process provided by pid
// Not supported on Windows (same as for docker daemon)
func GetOpenFds(pid int) (int, error) {
	return -1, nil
}
