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
