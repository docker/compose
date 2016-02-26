package runtime

import "os"

// TODO Windows: Linux uses syscalls which don't map to Windows. Needs alternate mechanism
func getExitPipe(path string) (*os.File, error) {
	return nil, nil
}

// TODO Windows: Linux uses syscalls which don't map to Windows. Needs alternate mechanism
func getControlPipe(path string) (*os.File, error) {
	return nil, nil
}

// TODO Windows. Windows does not support signals. Need alternate mechanism
// Signal sends the provided signal to the process
func (p *process) Signal(s os.Signal) error {
	return nil
}
