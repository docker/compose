//go:build !windows

/*
   Copyright 2023 Docker Compose CLI authors

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

package memnet

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

const maxUnixSocketPathSize = len(syscall.RawSockaddrUnix{}.Path)

func dialNamedPipe(_ context.Context, _ string) (net.Conn, error) {
	return nil, fmt.Errorf("named pipes are only available on Windows")
}

func validateSocketPath(addr string) error {
	if len(addr) > maxUnixSocketPathSize {
		return fmt.Errorf("socket address is too long: %s", addr)
	}
	return nil
}
