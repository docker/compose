package supervisor

import "github.com/rcrowley/go-metrics"

var (
	// ContainerCreateTimer holds the metrics timer associated with container creation
	ContainerCreateTimer = metrics.NewTimer()
	// ContainerDeleteTimer holds the metrics timer associated with container deletion
	ContainerDeleteTimer = metrics.NewTimer()
	// ContainerStartTimer holds the metrics timer associated with container start duration
	ContainerStartTimer = metrics.NewTimer()
	// ContainerStatsTimer holds the metrics timer associated with container stats generation
	ContainerStatsTimer = metrics.NewTimer()
	// ContainersCounter keeps track of the number of active containers
	ContainersCounter = metrics.NewCounter()
	// EventSubscriberCounter keeps track of the number of active event subscribers
	EventSubscriberCounter = metrics.NewCounter()
	// TasksCounter keeps track of the number of active supervisor tasks
	TasksCounter = metrics.NewCounter()
	// ExecProcessTimer holds the metrics timer associated with container exec
	ExecProcessTimer = metrics.NewTimer()
	// ExitProcessTimer holds the metrics timer associated with reporting container exit status
	ExitProcessTimer = metrics.NewTimer()
	// EpollFdCounter keeps trac of how many process are being monitored
	EpollFdCounter = metrics.NewCounter()
)

// Metrics return the list of all available metrics
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
