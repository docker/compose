package oci

import (
	"io"
	"os"

	"github.com/crosbymichael/go-runc"
)

type OIO struct {
	master  *os.File // master holds a fd to the created pty if any
	console string   // console holds the path the the slave linked to master
	rio     runc.IO  // rio holds the open fifos for stdios
}

func newOIO(stdin, stdout, stderr string, console bool) (o OIO, err error) {
	defer func() {
		if err != nil {
			o.cleanup()
		}
	}()

	if o.rio.Stdin, err = os.OpenFile(stdin, os.O_RDONLY, 0); err != nil {
		return
	}
	if o.rio.Stdout, err = os.OpenFile(stdout, os.O_WRONLY, 0); err != nil {
		return
	}
	if o.rio.Stderr, err = os.OpenFile(stderr, os.O_WRONLY, 0); err != nil {
		return
	}

	if console {
		o.master, o.console, err = newConsole(0, 0)
		if err != nil {
			return
		}
		go io.Copy(o.master, o.rio.Stdin)
		go func() {
			io.Copy(o.rio.Stdout, o.master)
			o.master.Close()
		}()
	}

	return
}

func (o OIO) cleanup() {
	if o.master != nil {
		o.master.Close()
	}
	if o.rio.Stdin != nil {
		o.rio.Stdin.(*os.File).Close()
	}
	if o.rio.Stdout != nil {
		o.rio.Stdout.(*os.File).Close()
	}
	if o.rio.Stderr != nil {
		o.rio.Stderr.(*os.File).Close()
	}
}
