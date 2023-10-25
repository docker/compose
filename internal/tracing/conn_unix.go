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

package tracing

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
)

const maxUnixSocketPathSize = len(syscall.RawSockaddrUnix{}.Path)

func DialInMemory(ctx context.Context, addr string) (net.Conn, error) {
	if !strings.HasPrefix(addr, "unix://") {
		return nil, fmt.Errorf("not a Unix socket address: %s", addr)
	}
	addr = strings.TrimPrefix(addr, "unix://")

	if len(addr) > maxUnixSocketPathSize {
		//goland:noinspection GoErrorStringFormat
		return nil, fmt.Errorf("Unix socket address is too long: %s", addr)
	}

	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}
