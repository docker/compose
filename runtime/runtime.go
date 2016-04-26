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
	ErrContainerStartTimeout = errors.New("containerd: container did not start before the specified timeout")

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

type Checkpoint struct {
	// Timestamp is the time that checkpoint happened
	Created time.Time `json:"created"`
	// Name is the name of the checkpoint
	Name string `json:"name"`
	// Tcp checkpoints open tcp connections
	Tcp bool `json:"tcp"`
	// UnixSockets persists unix sockets in the checkpoint
	UnixSockets bool `json:"unixSockets"`
	// Shell persists tty sessions in the checkpoint
	Shell bool `json:"shell"`
	// Exit exits the container after the checkpoint is finished
	Exit bool `json:"exit"`
}

// PlatformProcessState container platform-specific fields in the ProcessState structure
type PlatformProcessState struct {
	Checkpoint string `json:"checkpoint"`
	RootUID    int    `json:"rootUID"`
	RootGID    int    `json:"rootGID"`
}
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
	Bundle      string   `json:"bundle"`
	Labels      []string `json:"labels"`
	Stdin       string   `json:"stdin"`
	Stdout      string   `json:"stdout"`
	Stderr      string   `json:"stderr"`
	Runtime     string   `json:"runtime"`
	RuntimeArgs []string `json:"runtimeArgs"`
	Shim        string   `json:"shim"`
	NoPivotRoot bool     `json:"noPivotRoot"`
}

type ProcessState struct {
	specs.ProcessSpec
	Exec        bool     `json:"exec"`
	Stdin       string   `json:"containerdStdin"`
	Stdout      string   `json:"containerdStdout"`
	Stderr      string   `json:"containerdStderr"`
	RuntimeArgs []string `json:"runtimeArgs"`
	NoPivotRoot bool     `json:"noPivotRoot"`

	PlatformProcessState
}
