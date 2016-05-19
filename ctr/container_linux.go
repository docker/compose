package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

func createStdio() (s stdio, err error) {
	tmp, err := ioutil.TempDir("", "ctr-")
	if err != nil {
		return s, err
	}
	// create fifo's for the process
	for name, fd := range map[string]*string{
		"stdin":  &s.stdin,
		"stdout": &s.stdout,
		"stderr": &s.stderr,
	} {
		path := filepath.Join(tmp, name)
		if err := syscall.Mkfifo(path, 0755); err != nil && !os.IsExist(err) {
			return s, err
		}
		*fd = path
	}
	return s, nil
}
