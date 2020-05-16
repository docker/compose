// +build !windows

package server

import (
	"errors"
	"net"
	"strings"
)

func createLocalListener(address string) (net.Listener, error) {
	if !strings.HasPrefix(address, "unix://") {
		return nil, errors.New("Cannot parse address, must start with unix:// or tcp:// : " + address)
	}
	return net.Listen("unix", strings.TrimPrefix(address, "unix://"))
}
