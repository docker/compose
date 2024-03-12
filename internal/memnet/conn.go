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

package memnet

import (
	"context"
	"fmt"
	"net"
	"strings"
)

func DialEndpoint(ctx context.Context, endpoint string) (net.Conn, error) {
	if addr, ok := strings.CutPrefix(endpoint, "unix://"); ok {
		return Dial(ctx, "unix", addr)
	}
	if addr, ok := strings.CutPrefix(endpoint, "npipe://"); ok {
		return Dial(ctx, "npipe", addr)
	}
	return nil, fmt.Errorf("unsupported protocol for address: %s", endpoint)
}

func Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	switch network {
	case "unix":
		if err := validateSocketPath(addr); err != nil {
			return nil, err
		}
		return d.DialContext(ctx, "unix", addr)
	case "npipe":
		// N.B. this will return an error on non-Windows
		return dialNamedPipe(ctx, addr)
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
}
