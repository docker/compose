package execution

type Status string

const (
	Paused  Status = "paused"
	Running Status = "running"
	Stopped Status = "stopped"
	Deleted Status = "deleted"
)
