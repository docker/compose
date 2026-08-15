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
	"io"
	"slices"
	"strings"

	"github.com/morikuni/aec"
)

// screen owns the terminal region the progress block is painted on.
//
// It relies on a single contract with the layout: every line fits the
// terminal width, so one string is one terminal row and the cursor can move
// back over the block with plain relative moves. Repainting diffs against
// the previous frame: unchanged rows are skipped (a bare newline), changed
// rows are erased and rewritten, leftover rows from a taller previous frame
// are blanked. Each frame is flushed as a single Write.
type screen struct {
	out       io.Writer
	prev      []string
	prevWidth int
}

// reset forgets the painted block: the next paint starts fresh at the
// current cursor position. Used when foreign output (buildkit, a resize
// reflow) may have invalidated our notion of where the block lives.
func (s *screen) reset() {
	s.prev = nil
	s.prevWidth = 0
}

func (s *screen) paint(rows []string, width int) {
	if width < s.prevWidth {
		// The terminal shrank: previously painted rows may have wrapped and
		// the terminal may have reflowed them, so cursor arithmetic against
		// the old block is meaningless. Leave it behind and start a new one.
		s.reset()
	}
	redrawAll := width != s.prevWidth
	if !redrawAll && slices.Equal(rows, s.prev) {
		return // nothing changed, write nothing
	}

	var b strings.Builder
	b.WriteString(aec.Hide.String())
	if n := len(s.prev); n > 0 {
		b.WriteString(aec.Up(uint(n)).String())
	}
	b.WriteString(aec.Column(0).String())
	for i, row := range rows {
		if !redrawAll && i < len(s.prev) && s.prev[i] == row {
			b.WriteString("\n")
			continue
		}
		b.WriteString(aec.EraseLine(aec.EraseModes.All).String())
		b.WriteString(row)
		b.WriteString("\n")
	}
	if extra := len(s.prev) - len(rows); extra > 0 {
		for range extra {
			b.WriteString(aec.EraseLine(aec.EraseModes.All).String())
			b.WriteString("\n")
		}
		b.WriteString(aec.Up(uint(extra)).String())
	}
	b.WriteString(aec.Show.String())
	_, _ = io.WriteString(s.out, b.String())

	s.prev = rows
	s.prevWidth = width
}
