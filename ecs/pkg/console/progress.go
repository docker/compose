package console

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

type resource struct {
	name    string
	status  string
	details string
}

type progress struct {
	console   console
	resources []*resource
}

func NewProgressWriter() *progress {
	return &progress{
		console: ansiConsole{os.Stdout},
	}
}

const (
	cyan  = "36;1"
	red   = "31;1"
	green = "32;1"
)

func (p *progress) ResourceEvent(name string, status string, details string) {
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		logrus.Debugf("> %s : %s %s\n", name, status, details)
		return
	}
	p.console.MoveUp(len(p.resources))

	newResource := true
	for _, r := range p.resources {
		if r.name == name {
			newResource = false
			r.status = status
			r.details = details
			break
		}
	}
	if newResource {
		p.resources = append(p.resources, &resource{name, status, details})
	}

	var width int
	for _, r := range p.resources {
		l := len(r.name)
		if width < l {
			width = l
		}
	}

	for _, r := range p.resources {
		s := r.status
		if strings.HasSuffix(s, "_IN_PROGRESS") {
			s = p.console.WiP(s)
		} else if strings.HasSuffix(s, "_COMPLETE") {
			s = p.console.OK(s)
		} else if strings.HasSuffix(s, "_FAILED") {
			s = p.console.KO(s)
		}
		p.console.ClearLine()
		p.console.Printf("%-"+strconv.Itoa(width)+"s ... %s %s", r.name, s, r.details) // nolint:errcheck
		p.console.MoveDown(1)
	}
}

type console interface {
	Printf(format string, a ...interface{})
	MoveUp(int)
	MoveDown(int)
	ClearLine()
	OK(string) string
	KO(string) string
	WiP(string) string
}

type ansiConsole struct {
	out io.Writer
}

func (c ansiConsole) Printf(format string, a ...interface{}) {
	fmt.Fprintf(c.out, format, a...) // nolint:errcheck
	fmt.Fprintf(c.out, "\r")
}

func (c ansiConsole) MoveUp(i int) {
	if i == 0 {
		return
	}
	fmt.Fprintf(c.out, "\033[%dA", i) // nolint:errcheck
}

func (c ansiConsole) MoveDown(i int) {
	if i == 0 {
		return
	}
	fmt.Fprintf(c.out, "\033[%dB", i) // nolint:errcheck
}

func (c ansiConsole) ClearLine() {
	fmt.Fprint(c.out, "\033[2K\r") // nolint:errcheck
}

func (c ansiConsole) OK(s string) string {
	return ansiColor(green, s)
}

func (c ansiConsole) KO(s string) string {
	return ansiColor(red, s)
}

func (c ansiConsole) WiP(s string) string {
	return ansiColor(cyan, s)
}

func ansiColor(code, s string) string {
	return fmt.Sprintf("%s%s%s", ansi(code), s, ansi("0"))
}

func ansi(code string) string {
	return fmt.Sprintf("\033[%sm", code)
}
