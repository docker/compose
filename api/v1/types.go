package v1

import "time"

type State struct {
	Containers []Container `json:"containers"`
	Machine    Machine     `json:"machine"`
}

type Status string

const (
	Paused  Status = "paused"
	Running Status = "running"
)

type Machine struct {
	ID     string `json:"id"`
	Cpus   int    `json:"cpus"`
	Memory int64  `json:"memory"`
}

type ContainerState struct {
	Status Status `json:"status,omitempty"`
	Signal int    `json:"signal,omitempty"`
}

type Container struct {
	ID         string          `json:"id,omitempty"`
	BundlePath string          `json:"bundlePath,omitempty"`
	Processes  []Process       `json:"processes,omitempty"`
	Stdout     string          `json:"stdout,omitempty"`
	Stderr     string          `json:"stderr,omitempty"`
	State      *ContainerState `json:"state,omitempty"`
	Checkpoint string          `json:"checkpoint,omitempty"`
}

type User struct {
	UID            uint32   `json:"uid"`
	GID            uint32   `json:"gid"`
	AdditionalGids []uint32 `json:"additionalGids,omitempty"`
}

type Process struct {
	Terminal bool     `json:"terminal"`
	User     User     `json:"user"`
	Args     []string `json:"args,omitempty"`
	Env      []string `json:"env,omitempty"`
	Cwd      string   `json:"cwd,omitempty"`
	Pid      int      `json:"pid,omitempty"`
}

type Signal struct {
	Signal int `json:"signal"`
}

type Event struct {
	Type   string `json:"type"`
	ID     string `json:"id,omitempty"`
	Status int    `json:"status,omitempty"`
}

type Checkpoint struct {
	Name        string    `json:"name,omitempty"`
	Timestamp   time.Time `json:"timestamp,omitempty"`
	Exit        bool      `json:"exit,omitempty"`
	Tcp         bool      `json:"tcp"`
	UnixSockets bool      `json:"unixSockets"`
	Shell       bool      `json:"shell"`
}
