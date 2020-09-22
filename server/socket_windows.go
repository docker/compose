// +build windows

/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

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
