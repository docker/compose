package supervisor

import "time"

type StatsEvent struct {
	s *Supervisor
}

func (h *StatsEvent) Handle(e *Event) error {
	start := time.Now()
	i, ok := h.s.containers[e.ID]
	if !ok {
		return ErrContainerNotFound
	}
	// TODO: use workers for this
	go func() {
		s, err := i.container.Stats()
		if err != nil {
			e.Err <- err
			return
		}
		e.Err <- nil
		e.Stat <- s
		ContainerStatsTimer.UpdateSince(start)
	}()
	return errDeferedResponse
}
