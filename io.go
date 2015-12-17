package containerd

import (
	"io"
	"os"
)

type ioConfig struct {
	StdoutPath string
	StderrPath string
	StdinPath  string

	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

func newCopier(i *ioConfig) (*copier, error) {
	l := &copier{
		config: i,
	}
	if i.StdinPath != "" {
		f, err := os.OpenFile(i.StdinPath, os.O_RDONLY, 0)
		if err != nil {
			return nil, err
		}
		l.closers = append(l.closers, f)
		go func() {
			io.Copy(i.Stdin, f)
			i.Stdin.Close()
		}()
	}
	if i.StdoutPath != "" {
		f, err := os.OpenFile(i.StdoutPath, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		l.closers = append(l.closers, f)
		go io.Copy(f, i.Stdout)
	}
	if i.StderrPath != "" {
		f, err := os.OpenFile(i.StderrPath, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		l.closers = append(l.closers, f)
		go io.Copy(f, i.Stderr)
	}
	return l, nil
}

type copier struct {
	config  *ioConfig
	closers []io.Closer
}

func (l *copier) Close() (err error) {
	for _, c := range append(l.closers, l.config.Stdin, l.config.Stdout, l.config.Stderr) {
		if c != nil {
			if cerr := c.Close(); err == nil {
				err = cerr
			}
		}
	}
	return err
}
