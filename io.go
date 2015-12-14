package containerd

import (
	"io"
	"os"
)

type ioConfig struct {
	StdoutPath string
	StderrPath string
	Stdin      io.WriteCloser
	Stdout     io.ReadCloser
	Stderr     io.ReadCloser
}

func newCopier(i *ioConfig) (*copier, error) {
	l := &copier{
		config: i,
	}
	if i.StdoutPath != "" {
		f, err := os.OpenFile(i.StdoutPath, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		go io.Copy(f, i.Stdout)
	}
	if i.StderrPath != "" {
		f, err := os.OpenFile(i.StderrPath, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		go io.Copy(f, i.Stderr)
	}
	return l, nil
}

type copier struct {
	config *ioConfig
}

func (l *copier) Close() (err error) {
	for _, c := range []io.Closer{
		l.config.Stdin,
		l.config.Stdout,
		l.config.Stderr,
	} {
		if cerr := c.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
