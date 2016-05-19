package supervisor

type platformStartTask struct {
}

// Checkpoint not supported on Solaris
func (task *startTask) setTaskCheckpoint(t *StartTask) {
}
