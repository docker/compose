package v1

type State struct {
	Containers []Container `json:"containers"`
}

type Container struct {
	ID         string `json:"id,omitempty"`
	BundlePath string `json:"bundlePath,omitempty"`
	Processes  []int  `json:"processes,omitempty"`
}

type Signal struct {
	Signal int `json:"signal"`
}
