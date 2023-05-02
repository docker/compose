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

	"github.com/Microsoft/go-winio"
)

func DialInMemory(ctx context.Context, addr string) (net.Conn, error) {
	if !strings.HasPrefix(addr, "npipe://") {
		return nil, fmt.Errorf("not a named pipe address: %s", addr)
	}
	addr = strings.TrimPrefix(addr, "npipe://")

	return winio.DialPipeContext(ctx, addr)
}
