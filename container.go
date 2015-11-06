package containerd

type Container interface {
	ID() string
	Start() error
	Pid() (int, error)
	//	Process() Process
	SetExited(status int)
	Delete() error
}
