import "time"

type TimedEvent struct {
	eventName string
	duration  time.Duration
}

type timedEventSorter struct {
	events []TimedEvent
	by     func(te1, te2 *TimedEvent) bool
}

func (s *timedEventSorter) Len() int {
	return len(s.events)
}

func (s *timedEventSorter) Swap(i, j int) {
	s.events[i], s.events[j] = s.events[j], s.events[i]
}

func (s *timedEventSorter) Less(i, j int) bool {
	return s.by(&s.events[i], &s.events[j])
}
