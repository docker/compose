package supervisor

type platformStartTask struct {
}

// Checkpoint not supported on Windows
func (task *startTask) setTaskCheckpoint(t *StartTask) {
}
