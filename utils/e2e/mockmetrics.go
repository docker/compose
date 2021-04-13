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

package e2e

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo"
)

// MockMetricsServer a mock registring all metrics POST invocations
type MockMetricsServer struct {
	socket string
	usage  []string
	e      *echo.Echo
}

// NewMetricsServer instaniate a new MockMetricsServer
func NewMetricsServer(socket string) *MockMetricsServer {
	return &MockMetricsServer{
		socket: socket,
		e:      echo.New(),
	}
}

func (s *MockMetricsServer) handlePostUsage(c echo.Context) error {
	body, error := ioutil.ReadAll(c.Request().Body)
	if error != nil {
		return error
	}
	cliUsage := string(body)
	s.usage = append(s.usage, cliUsage)
	return c.String(http.StatusOK, "")
}

// GetUsage get usage
func (s *MockMetricsServer) GetUsage() []string {
	return s.usage
}

// ResetUsage reset usage
func (s *MockMetricsServer) ResetUsage() {
	s.usage = []string{}
}

// Stop stop the mock server
func (s *MockMetricsServer) Stop() {
	_ = s.e.Close()
}

// Start start the mock server
func (s *MockMetricsServer) Start() {
	go func() {
		listener, err := net.Listen("unix", strings.TrimPrefix(s.socket, "unix://"))
		if err != nil {
			log.Fatal(err)
		}
		s.e.Listener = listener
		s.e.POST("/usage", s.handlePostUsage)
		_ = s.e.Start(":1323")
	}()
}
