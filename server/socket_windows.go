// +build windows

package server

import (
	"errors"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
)

func createLocalListener(address string) (net.Listener, error) {
	if !strings.HasPrefix(address, "npipe://") {
		return nil, errors.New("Cannot parse address, must start with npipe:// or tcp:// : " + address)
	}
	return winio.ListenPipe(strings.TrimPrefix(address, "npipe://"), &winio.PipeConfig{
		MessageMode:      true,  // Use message mode so that CloseWrite() is supported
		InputBufferSize:  65536, // Use 64KB buffers to improve performance
		OutputBufferSize: 65536,
	})
}
