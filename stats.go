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

type StatsEvent struct {
	s *Supervisor
}

type UnsubscribeStatsEvent struct {
	s *Supervisor
}

type StopStatsEvent struct {
	s *Supervisor
}

func (h *StatsEvent) Handle(e *Event) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	e.Stats = h.s.statsCollector.collect(i.container)
	return nil
}

func (h *UnsubscribeStatsEvent) Handle(e *Event) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	h.s.statsCollector.unsubscribe(i.container, e.Stats)
	return nil
}

func (h *StopStatsEvent) Handle(e *Event) error {
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	h.s.statsCollector.stopCollection(i.container)
	return nil
}
