// +build linux

package main

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/docker/docker/pkg/term"
	"github.com/opencontainers/runc/libcontainer"
)

// newTty creates a new tty for use with the container.  If a tty is not to be
// created for the process, pipes are created so that the TTY of the parent
// process are not inherited by the container.
func newTty(create bool, p *libcontainer.Process, rootuid int, console string) (*tty, error) {
	if create {
		return createTty(p, rootuid, console)
	}
	return createStdioPipes(p, rootuid)
}

// setup standard pipes so that the TTY of the calling runc process
// is not inherited by the container.
func createStdioPipes(p *libcontainer.Process, rootuid int) (*tty, error) {
	i, err := p.InitializeIO(rootuid)
	if err != nil {
		return nil, err
	}
	t := &tty{
		closers: []io.Closer{
			i.Stdin,
			i.Stdout,
			i.Stderr,
		},
	}
	// add the process's io to the post start closers if they support close
	for _, cc := range []interface{}{
		p.Stdin,
		p.Stdout,
		p.Stderr,
	} {
		if c, ok := cc.(io.Closer); ok {
			t.postStart = append(t.postStart, c)
		}
	}
	go func() {
		io.Copy(i.Stdin, os.Stdin)
		i.Stdin.Close()
	}()
	t.wg.Add(2)
	go t.copyIO(os.Stdout, i.Stdout)
	go t.copyIO(os.Stderr, i.Stderr)
	return t, nil
}

func (t *tty) copyIO(w io.Writer, r io.ReadCloser) {
	defer t.wg.Done()
	io.Copy(w, r)
	r.Close()
}

func createTty(p *libcontainer.Process, rootuid int, consolePath string) (*tty, error) {
	if consolePath != "" {
		if err := p.ConsoleFromPath(consolePath); err != nil {
			return nil, err
		}
		return &tty{}, nil
	}
	console, err := p.NewConsole(rootuid)
	if err != nil {
		return nil, err
	}
	go io.Copy(console, os.Stdin)
	go io.Copy(os.Stdout, console)

	state, err := term.SetRawTerminal(os.Stdin.Fd())
	if err != nil {
		return nil, fmt.Errorf("failed to set the terminal from the stdin: %v", err)
	}
	return &tty{
		console: console,
		state:   state,
		closers: []io.Closer{
			console,
		},
	}, nil
}

type tty struct {
	console   libcontainer.Console
	state     *term.State
	closers   []io.Closer
	postStart []io.Closer
	wg        sync.WaitGroup
}

// ClosePostStart closes any fds that are provided to the container and dup2'd
// so that we no longer have copy in our process.
func (t *tty) ClosePostStart() error {
	for _, c := range t.postStart {
		c.Close()
	}
	return nil
}

// Close closes all open fds for the tty and/or restores the orignal
// stdin state to what it was prior to the container execution
func (t *tty) Close() error {
	// ensure that our side of the fds are always closed
	for _, c := range t.postStart {
		c.Close()
	}
	// wait for the copy routines to finish before closing the fds
	t.wg.Wait()
	for _, c := range t.closers {
		c.Close()
	}
	if t.state != nil {
		term.RestoreTerminal(os.Stdin.Fd(), t.state)
	}
	return nil
}

func (t *tty) resize() error {
	if t.console == nil {
		return nil
	}
	ws, err := term.GetWinsize(os.Stdin.Fd())
	if err != nil {
		return err
	}
	return term.SetWinsize(t.console.Fd(), ws)
}
