package main

import (
	"errors"
)

func createStdio() (s stdio, err error) {
	return s, errors.New("createStdio not implemented on Solaris")
}
