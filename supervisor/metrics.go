package supervisor

import "github.com/rcrowley/go-metrics"

var (
	ContainerCreateTimer   = metrics.NewTimer()
	ContainerDeleteTimer   = metrics.NewTimer()
	ContainerStartTimer    = metrics.NewTimer()
	ContainerStatsTimer    = metrics.NewTimer()
	ContainersCounter      = metrics.NewCounter()
	EventSubscriberCounter = metrics.NewCounter()
	TasksCounter           = metrics.NewCounter()
	ExecProcessTimer       = metrics.NewTimer()
	ExitProcessTimer       = metrics.NewTimer()
	EpollFdCounter         = metrics.NewCounter()
)

func Metrics() map[string]interface{} {
	return map[string]interface{}{
		"container-create-time": ContainerCreateTimer,
		"container-delete-time": ContainerDeleteTimer,
		"container-start-time":  ContainerStartTimer,
		"container-stats-time":  ContainerStatsTimer,
		"containers":            ContainersCounter,
		"event-subscribers":     EventSubscriberCounter,
		"tasks":                 TasksCounter,
		"exec-process-time":     ExecProcessTimer,
		"exit-process-time":     ExitProcessTimer,
		"epoll-fds":             EpollFdCounter,
	}
}
