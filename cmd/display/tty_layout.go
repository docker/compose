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
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/mattn/go-runewidth"

	"github.com/docker/compose/v5/pkg/api"
)

// The layout is a pure function of (taskTree, operation, layoutOpts):
// no clocks, no terminal, no mutation. Its single hard invariant, enforced
// by renderSegs, is that every returned line occupies at most o.width
// terminal cells. This is what makes screen cursor arithmetic sound: a line
// that never wraps is a line the cursor can reliably move back over.
//
// All width computations happen on plain text, in terminal cells (via
// go-runewidth, so CJK and other wide runes count as 2); colors are applied
// only when a segment's final text is known. ANSI sequences therefore never
// enter any measurement.

// seg is a piece of a row: plain text plus the color to apply once its
// geometry is final.
type seg struct {
	text  string
	color colorFunc
}

type layoutOpts struct {
	width, height int
	dryRun        bool
	now           time.Time
}

const (
	tickInterval = 100 * time.Millisecond
	minIDCells   = 10
	// left margin: space + spinner + space
	marginCells = 3
	// widest status text worth reserving column space for; longer ones are
	// truncated rather than allowed to squeeze the id column
	maxStatusReserveCells = 20
)

// layoutFrame renders the whole progress block, one string per terminal row.
func layoutFrame(t *taskTree, operation string, o layoutOpts) []string {
	done, total := t.counts()
	lines := []string{fmt.Sprintf("[+] %s %d/%d", operation, done, total)}

	rows := buildRows(t, o)
	maxRows := max(o.height-2, 1)
	var more int
	if len(rows) > maxRows {
		more = len(rows) - (maxRows - 1)
		rows = rows[:maxRows-1]
	}

	cols := computeColumns(rows, o.width)
	for i := range rows {
		lines = append(lines, renderSegs(rowSegs(&rows[i], cols, o.width), o.width))
	}
	if more > 0 {
		lines = append(lines, renderSegs([]seg{{text: fmt.Sprintf(" ... %d more", more)}}, o.width))
	}
	return lines
}

// row is the pre-rendered content of one line, before column fitting.
type row struct {
	spin        seg
	prefix      string // dry-run prefix
	id          string
	barStrip    string // aggregated braille strip, one glyph per child task
	sizes       string // droppable progress suffix appended after the bar
	status      string
	statusColor colorFunc
	details     string
	timer       string
}

func buildRows(t *taskTree, o layoutOpts) []row {
	var rows []row
	for _, root := range t.roots() {
		rows = append(rows, makeRow(t, root, o))
	}
	return rows
}

func makeRow(t *taskTree, n *node, o layoutOpts) row {
	r := row{
		spin:        spinGlyph(n, o.now),
		id:          n.id,
		status:      n.text,
		statusColor: colorFn(n.status),
		details:     n.details,
		timer:       fmt.Sprintf("%.1fs", nodeElapsed(n, o.now).Seconds()),
	}
	if o.dryRun {
		r.prefix = DRYRUN_PREFIX
	}
	r.barStrip, r.sizes = aggregateProgress(t, n)
	return r
}

func nodeElapsed(n *node, now time.Time) time.Duration {
	switch {
	case n.status == api.Working:
		return now.Sub(n.startedAt)
	case !n.endedAt.IsZero():
		return n.endedAt.Sub(n.startedAt)
	default:
		return 0
	}
}

// aggregateProgress compresses the children of a root task into a braille
// strip (one glyph per child) and a "current / total" size suffix.
func aggregateProgress(t *taskTree, root *node) (strip, sizes string) {
	if root.status != api.Working {
		return "", ""
	}
	var (
		total, current int64
		hideSizes      bool
		glyphs         []string
	)
	for _, child := range t.children(root.id) {
		if child.status == api.Working && child.total == 0 {
			hideSizes = true
		}
		total += child.total
		current += child.current
		r := len(percentChars) - 1
		p := min(child.percent, 100)
		glyphs = append(glyphs, percentChars[r*p/100])
	}
	if len(glyphs) == 0 {
		return "", ""
	}
	if total == 0 {
		hideSizes = true
	}
	if !hideSizes {
		sizes = fmt.Sprintf(" %7s / %-7s", units.HumanSize(float64(current)), units.HumanSize(float64(total)))
	}
	return strings.Join(glyphs, ""), sizes
}

// columns holds the shared geometry aligning all rows of a frame.
type columns struct {
	left  int // indent + prefix + id + bar + sizes
	timer int
}

func computeColumns(rows []row, width int) columns {
	var c columns
	var status int
	for i := range rows {
		r := &rows[i]
		c.timer = max(c.timer, runewidth.StringWidth(r.timer))
		c.left = max(c.left, leftWidth(r, true))
		status = max(status, runewidth.StringWidth(r.status))
	}
	// keep room for the widest (capped) status and the filler before the timer
	reserve := min(status, maxStatusReserveCells) + 2
	c.left = min(c.left, max(width-marginCells-reserve-c.timer, minIDCells))
	return c
}

// leftWidth measures the left block of a row in cells.
func leftWidth(r *row, withSizes bool) int {
	w := runewidth.StringWidth(r.prefix) + runewidth.StringWidth(r.id)
	if r.barStrip != "" {
		w += 3 + runewidth.StringWidth(r.barStrip) // " [" + strip + "]"
	}
	if withSizes {
		w += runewidth.StringWidth(r.sizes)
	}
	return w
}

// rowSegs lays one row out into segments. Content degrades in a fixed order
// when space is short: drop the sizes suffix, truncate the id, truncate
// status, truncate then drop details. renderSegs is the final hard guarantee.
func rowSegs(r *row, c columns, width int) []seg {
	var segs []seg
	used := 0
	add := func(text string, color colorFunc) {
		if text == "" {
			return
		}
		segs = append(segs, seg{text: text, color: color})
		used += runewidth.StringWidth(text)
	}

	add(" ", nil)
	add(r.spin.text, r.spin.color)
	add(r.prefix, PrefixColor)
	add(" ", nil)

	// left block: id + bar + sizes, fitted to the shared left column
	sizes := r.sizes
	if leftWidth(r, true) > c.left {
		sizes = "" // drop the size suffix first
	}
	if lw := leftWidth(r, false); lw > c.left {
		idBudget := max(c.left-(lw-runewidth.StringWidth(r.id)), minIDCells-3)
		r.id = truncateCells(r.id, idBudget)
	}
	add(r.id, nil)
	if r.barStrip != "" {
		add(" [", nil)
		add(r.barStrip, SuccessColor)
		add("]", nil)
	}
	add(sizes, nil)
	if pad := marginCells + c.left - used; pad > 0 {
		add(strings.Repeat(" ", pad), nil)
	}

	// status, then details, each within what remains before the timer
	if budget := width - used - c.timer - 2; budget > 0 {
		add(" ", nil)
		add(truncateCells(r.status, budget), r.statusColor)
	}
	if budget := width - used - c.timer - 2; r.details != "" && budget >= 5 {
		add(" ", nil)
		add(truncateCells(r.details, budget-1), nil)
	}

	// right-aligned timer
	if fill := width - used - c.timer; fill > 0 {
		add(strings.Repeat(" ", fill), nil)
	}
	add(strings.Repeat(" ", max(c.timer-runewidth.StringWidth(r.timer), 0)), nil)
	add(r.timer, TimerColor)
	return segs
}

// renderSegs assembles segments into the final styled line, enforcing the
// package invariant: the rendered line occupies at most maxCells terminal
// cells. Truncation happens on plain text before coloring, so the clip is
// both ANSI-safe and UTF-8-safe.
func renderSegs(segs []seg, maxCells int) string {
	var sb strings.Builder
	cells := 0
	for _, s := range segs {
		w := runewidth.StringWidth(s.text)
		if cells+w > maxCells {
			s.text = runewidth.Truncate(s.text, maxCells-cells, "")
			w = runewidth.StringWidth(s.text)
		}
		if s.text != "" {
			if s.color != nil {
				sb.WriteString(s.color(s.text))
			} else {
				sb.WriteString(s.text)
			}
			cells += w
		}
		if cells >= maxCells {
			break
		}
	}
	return sb.String()
}

// truncateCells shortens s to the given number of terminal cells, appending
// "..." when something was actually cut and space allows.
func truncateCells(s string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= cells {
		return s
	}
	if cells <= 3 {
		return runewidth.Truncate(s, cells, "")
	}
	return runewidth.Truncate(s, cells, "...")
}

var (
	spinnerDone    = "✔"
	spinnerWarning = "!"
	spinnerError   = "✘"

	// percentChars maps a completion ratio to a braille glyph for the
	// aggregated per-image strip
	percentChars = strings.Split("⠀⡀⣀⣄⣤⣦⣶⣷⣿", "")

	termSpinnerFrames = spinnerFrames()
)

func colorFn(s api.EventStatus) colorFunc {
	switch s {
	case api.Done:
		return SuccessColor
	case api.Warning:
		return WarningColor
	case api.Error:
		return ErrorColor
	default:
		return nocolor
	}
}

func spinnerFrames() []string {
	if runtime.GOOS == "windows" {
		return []string{"-"}
	}
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
}

// spinGlyph derives the per-row glyph from status and time. The animation
// frame is a pure function of the clock, so its speed doesn't depend on how
// often the layout runs.
func spinGlyph(n *node, now time.Time) seg {
	switch n.status {
	case api.Done:
		return seg{text: spinnerDone, color: SuccessColor}
	case api.Warning:
		return seg{text: spinnerWarning, color: WarningColor}
	case api.Error:
		return seg{text: spinnerError, color: ErrorColor}
	default:
		frame := int(now.Sub(n.startedAt)/tickInterval) % len(termSpinnerFrames)
		if frame < 0 {
			frame = 0
		}
		return seg{text: termSpinnerFrames[frame], color: CountColor}
	}
}
