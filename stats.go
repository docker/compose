package containerd

import "github.com/rcrowley/go-metrics"

var (
	ContainerStartTimer    = metrics.NewTimer()
	ContainersCounter      = metrics.NewCounter()
	EventsCounter          = metrics.NewCounter()
	EventSubscriberCounter = metrics.NewCounter()
)

func Metrics() map[string]interface{} {
	return map[string]interface{}{
		"container-start-time": ContainerStartTimer,
		"containers":           ContainersCounter,
		"events":               EventsCounter,
		"events-subscribers":   EventSubscriberCounter,
	}
}
