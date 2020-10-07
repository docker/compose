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

package metrics

import (
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

var (
	socket = `\\.\pipe\docker_cli`
)

func conn() (net.Conn, error) {
	if strings.HasPrefix(socket, `\\.\pipe\`) {
		timeout := 200 * time.Millisecond
		return winio.DialPipe(socket, &timeout)
	}
	return net.Dial("unix", socket)
}
