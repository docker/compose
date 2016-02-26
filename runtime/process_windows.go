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

func populateProcessStateForEncoding(config *processConfig, uid int, gid int) ProcessState {
	return ProcessState{
		ProcessSpec: config.processSpec,
		Exec:        config.exec,
		Stdin:       config.stdio.Stdin,
		Stdout:      config.stdio.Stdout,
		Stderr:      config.stdio.Stderr,
	}
}
