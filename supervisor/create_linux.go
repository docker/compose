package supervisor

import "github.com/docker/containerd/runtime"

type platformStartTask struct {
	Checkpoint *runtime.Checkpoint
}

func (task *startTask) setTaskCheckpoint(t *StartTask) {
	if t.Checkpoint != nil {
		task.Checkpoint = t.Checkpoint.Name
	}
}
