/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package display

import (
	"runtime"
	"time"
)

// Spinner renders an animated terminal spinner to indicate ongoing progress.
type Spinner struct {
	time  time.Time
	index int
	chars []string
	stop  bool
	done  string
}

// NewSpinner creates and returns a new Spinner instance with platform-appropriate characters.
func NewSpinner() *Spinner {
	chars := []string{
		"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
	}
	done := "⠿"

	if runtime.GOOS == "windows" {
		chars = []string{"-"}
		done = "-"
	}

	return &Spinner{
		index: 0,
		time:  time.Now(),
		chars: chars,
		done:  done,
	}
}

func (s *Spinner) String() string {
	if s.stop {
		return s.done
	}

	d := time.Since(s.time)
	if d.Milliseconds() > 100 {
		s.index = (s.index + 1) % len(s.chars)
	}

	return s.chars[s.index]
}

// Stop marks the spinner as done, causing it to display the completion character.
func (s *Spinner) Stop() {
	s.stop = true
}

// Restart resumes the spinner animation after it has been stopped.
func (s *Spinner) Restart() {
	s.stop = false
}
