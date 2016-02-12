package supervisor

import (
	"os"
	"time"

	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/specs"
)

type TaskType string

const (
	ExecExitTaskType         TaskType = "execExit"
	ExitTaskType             TaskType = "exit"
	StartContainerTaskType   TaskType = "startContainer"
	DeleteTaskType           TaskType = "deleteContainerEvent"
	GetContainerTaskType     TaskType = "getContainer"
	SignalTaskType           TaskType = "signal"
	AddProcessTaskType       TaskType = "addProcess"
	UpdateContainerTaskType  TaskType = "updateContainer"
	UpdateProcessTaskType    TaskType = "updateProcess"
	CreateCheckpointTaskType TaskType = "createCheckpoint"
	DeleteCheckpointTaskType TaskType = "deleteCheckpoint"
	StatsTaskType            TaskType = "events"
	OOMTaskType              TaskType = "oom"
)

func NewTask(t TaskType) *Task {
	return &Task{
		Type:      t,
		Timestamp: time.Now(),
		Err:       make(chan error, 1),
	}
}

type StartResponse struct {
	Container runtime.Container
}

type Task struct {
	Type          TaskType
	Timestamp     time.Time
	ID            string
	BundlePath    string
	Stdout        string
	Stderr        string
	Stdin         string
	Console       string
	Pid           string
	Status        int
	Signal        os.Signal
	Process       runtime.Process
	State         runtime.State
	ProcessSpec   *specs.Process
	Containers    []runtime.Container
	Checkpoint    *runtime.Checkpoint
	Err           chan error
	StartResponse chan StartResponse
	Stat          chan *runtime.Stat
	CloseStdin    bool
	ResizeTty     bool
	Width         int
	Height        int
	Labels        []string
}

type Handler interface {
	Handle(*Task) error
}

type commonTask struct {
	data *Task
	sv   *Supervisor
}

func (e *commonTask) Handle() {
	h, ok := e.sv.handlers[e.data.Type]
	if !ok {
		e.data.Err <- ErrUnknownTask
		return
	}
	err := h.Handle(e.data)
	if err != errDeferedResponse {
		e.data.Err <- err
		close(e.data.Err)
		return
	}
}
