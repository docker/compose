package runtime

import (
	"errors"
	"time"

	"github.com/docker/containerd/specs"
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
	ErrContainerNotStarted   = errors.New("containerd: container not started")

	errNoPidFile      = errors.New("containerd: no process pid file found")
	errInvalidPidInt  = errors.New("containerd: process pid is invalid")
	errNotImplemented = errors.New("containerd: not implemented")
)

const (
	ExitFile       = "exit"
	ExitStatusFile = "exitStatus"
	StateFile      = "state.json"
	ControlFile    = "control"
	InitProcessID  = "init"
)

type State string

type Resource struct {
	CPUShares         int64
	BlkioWeight       uint16
	CPUPeriod         int64
	CPUQuota          int64
	CpusetCpus        string
	CpusetMems        string
	KernelMemory      int64
	Memory            int64
	MemoryReservation int64
	MemorySwap        int64
}

const (
	Paused  = State("paused")
	Stopped = State("stopped")
	Running = State("running")
)

type state struct {
	Bundle  string   `json:"bundle"`
	Labels  []string `json:"labels"`
	Stdin   string   `json:"stdin"`
	Stdout  string   `json:"stdout"`
	Stderr  string   `json:"stderr"`
	Runtime string   `json:"runtime"`
}

type ProcessState struct {
	specs.ProcessSpec
	Exec   bool   `json:"exec"`
	Stdin  string `json:"containerdStdin"`
	Stdout string `json:"containerdStdout"`
	Stderr string `json:"containerdStderr"`

	PlatformProcessState
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
