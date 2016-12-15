package execution

type Status string

const (
	Created Status = "created"
	Paused  Status = "paused"
	Running Status = "running"
	Stopped Status = "stopped"
	Deleted Status = "deleted"
)
