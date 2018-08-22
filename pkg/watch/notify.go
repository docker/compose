package watch

type FileEvent struct {
	Path string
}

type Notify interface {
	Close() error
	Add(name string) error
	Events() chan FileEvent
	Errors() chan error
}
