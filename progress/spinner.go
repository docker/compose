package progress

import "time"

type spinner struct {
	time  time.Time
	index int
	chars []string
	stop  bool
	done  string
}

func newSpinner() *spinner {
	return &spinner{
		index: 0,
		time:  time.Now(),
		chars: []string{
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		},
		done: "⠿",
	}
}

func (s *spinner) String() string {
	if s.stop {
		return s.done
	}

	d := time.Since(s.time)
	if d.Milliseconds() > 100 {
		s.index = (s.index + 1) % len(s.chars)
	}

	return s.chars[s.index]
}

func (s *spinner) Stop() {
	s.stop = true
}
