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

package api

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestPortPublisherString(t *testing.T) {
	tests := []struct {
		name string
		pub  PortPublisher
		want string
	}{
		{"ipv4", PortPublisher{URL: "0.0.0.0", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"}, "80/tcp -> 0.0.0.0:8080"},
		{"ipv6", PortPublisher{URL: "::", TargetPort: 5060, PublishedPort: 32769, Protocol: "udp"}, "5060/udp -> [::]:32769"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.pub.String(), tt.want)
		})
	}
}

func TestRunOptionsEnvironmentMap(t *testing.T) {
	opts := RunOptions{
		Environment: []string{
			"FOO=BAR",
			"ZOT=",
			"QIX",
		},
	}
	env := types.NewMappingWithEquals(opts.Environment)
	assert.Equal(t, *env["FOO"], "BAR")
	assert.Equal(t, *env["ZOT"], "")
	assert.Check(t, env["QIX"] == nil)
}
