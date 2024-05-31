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

func TestPortPublisherString(t *testing.T) {
	tests := map[string]struct {
		pub    PortPublisher
		expect string
	}{
		"ipv6_udp": {
			pub: PortPublisher{
				Protocol:      "udp",
				PublishedPort: 32769,
				TargetPort:    5060,
				URL:           "::",
			},
			expect: "5060/udp -> [::]:32769",
		},
		"ipv4_tcp": {
			pub: PortPublisher{
				Protocol:      "tcp",
				PublishedPort: 5060,
				TargetPort:    5060,
				URL:           "0.0.0.0",
			},
			expect: "5060/tcp -> 0.0.0.0:5060",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.pub.String()
			assert.Equal(t, tt.expect, got)
		})
	}
}
