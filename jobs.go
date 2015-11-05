package containerd

type Job interface {
}

type CreateJob struct {
	Err chan error
}
