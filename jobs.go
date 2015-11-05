package containerd

type Job interface {
}

type CreateJob struct {
	ID         string
	BundlePath string
	Err        chan error
}
