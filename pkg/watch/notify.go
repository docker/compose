package watch

import "github.com/windmilleng/fsnotify"

type Notify interface {
	Close() error
	Add(name string) error
	Events() chan fsnotify.Event
	Errors() chan error
}
