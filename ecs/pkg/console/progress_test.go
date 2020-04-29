package console

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestProgressWriter(t *testing.T) {
	c := &bufferConsole{}
	p := progress{
		console: c,
	}
	p.ResourceEvent("resource1", "CREATE_IN_PROGRESS", "")
	assert.Equal(t, c.lines[0], "resource1 ... CREATE_IN_PROGRESS ")

	p.ResourceEvent("resource2_long_name", "CREATE_IN_PROGRESS", "ok")
	assert.Equal(t, c.lines[0], "resource1           ... CREATE_IN_PROGRESS ")
	assert.Equal(t, c.lines[1], "resource2_long_name ... CREATE_IN_PROGRESS ok")

	p.ResourceEvent("resource2_long_name", "CREATE_COMPLETE", "done")
	assert.Equal(t, c.lines[0], "resource1           ... CREATE_IN_PROGRESS ")
	assert.Equal(t, c.lines[1], "resource2_long_name ... CREATE_COMPLETE done")

	p.ResourceEvent("resource1", "CREATE_FAILED", "oups")
	assert.Equal(t, c.lines[0], "resource1           ... CREATE_FAILED oups")
	assert.Equal(t, c.lines[1], "resource2_long_name ... CREATE_COMPLETE done")
}

type bufferConsole struct {
	pos   int
	lines []string
}

func (b *bufferConsole) Printf(format string, a ...interface{}) {
	b.lines[b.pos] = fmt.Sprintf(format, a...)
}

func (b *bufferConsole) MoveUp(i int) {
	b.pos -= i
}

func (b *bufferConsole) MoveDown(i int) {
	b.pos += i
}

func (b *bufferConsole) ClearLine() {
	if len(b.lines) <= b.pos {
		b.lines = append(b.lines, "")
	}
	b.lines[b.pos] = ""
}

func (b *bufferConsole) OK(s string) string {
	return s
}

func (b *bufferConsole) KO(s string) string {
	return s
}

func (b *bufferConsole) WiP(s string) string {
	return s
}
