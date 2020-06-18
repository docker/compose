package progress

import (
	"runtime"
	"time"
)

type spinner struct {
	time  time.Time
	index int
	chars []string
	stop  bool
	done  string
}

func newSpinner() *spinner {
	chars := []string{
		"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
	}
	done := "⠿"

	if runtime.GOOS == "windows" {
		chars = []string{"-"}
		done = "-"
	}

	return &spinner{
		index: 0,
		time:  time.Now(),
		chars: chars,
		done:  done,
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
