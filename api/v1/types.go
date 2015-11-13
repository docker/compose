package v1

type State struct {
	Containers []Container `json:"containers"`
}

type Status string

const (
	Paused  Status = "paused"
	Running Status = "running"
)

type ContainerState struct {
	Status Status `json:"status,omitempty"`
}

type Container struct {
	ID         string          `json:"id,omitempty"`
	BundlePath string          `json:"bundlePath,omitempty"`
	Processes  []Process       `json:"processes,omitempty"`
	Stdout     string          `json:"stdout,omitempty"`
	Stderr     string          `json:"stderr,omitempty"`
	State      *ContainerState `json:"state,omitempty"`
}

type User struct {
	UID            uint32   `json:"uid"`
	GID            uint32   `json:"gid"`
	AdditionalGids []uint32 `json:"additionalGids,omitempty"`
}

type Process struct {
	Terminal bool     `json:"terminal,omitempty"`
	User     User     `json:"user,omitempty"`
	Args     []string `json:"args,omitempty"`
	Env      []string `json:"env,omitempty"`
	Cwd      string   `json:"cwd,omitempty"`
	Pid      int      `json:"pid,omitempty"`
}

type Signal struct {
	Signal int `json:"signal"`
}
