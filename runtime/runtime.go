package runtime

import (
	"errors"
	"time"
)

var (
	ErrNotChildProcess       = errors.New("containerd: not a child process for container")
	ErrInvalidContainerType  = errors.New("containerd: invalid container type for runtime")
	ErrCheckpointNotExists   = errors.New("containerd: checkpoint does not exist for container")
	ErrCheckpointExists      = errors.New("containerd: checkpoint already exists")
	ErrContainerExited       = errors.New("containerd: container has exited")
	ErrTerminalsNotSupported = errors.New("containerd: terminals are not supported for runtime")
	ErrProcessNotExited      = errors.New("containerd: process has not exited")
	ErrProcessExited         = errors.New("containerd: process has exited")

	errNotImplemented = errors.New("containerd: not implemented")
)

const (
	ExitFile       = "exit"
	ExitStatusFile = "exitStatus"
	StateFile      = "state.json"
	InitProcessID  = "init"
)

type State string

const (
	Paused  = State("paused")
	Running = State("running")
)

type state struct {
	Bundle string `json:"bundle"`
}

type Stat struct {
	// Timestamp is the time that the statistics where collected
	Timestamp time.Time
	// Data is the raw stats
	// TODO: it is currently an interface because we don't know what type of exec drivers
	// we will have or what the structure should look like at the moment os the containers
	// can return what they want and we could marshal to json or whatever.
	Data interface{}
}

type Checkpoint struct {
	// Timestamp is the time that checkpoint happened
	Timestamp time.Time
	// Name is the name of the checkpoint
	Name string
	// Tcp checkpoints open tcp connections
	Tcp bool
	// UnixSockets persists unix sockets in the checkpoint
	UnixSockets bool
	// Shell persists tty sessions in the checkpoint
	Shell bool
	// Exit exits the container after the checkpoint is finished
	Exit bool
}
