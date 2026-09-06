/*
   Copyright 2026 Docker Compose CLI authors

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

package main

import (
	"reflect"
	"testing"
)

// A provider announces endpoints as seen from its host; the relay, which
// knows it runs inside a container, rewrites host-local upstreams to
// host.docker.internal and leaves routable addresses untouched.
func TestParseRoutesTranslatesHostLocalUpstreams(t *testing.T) {
	routes, err := parseRoutes("80=localhost:49152,81=127.0.0.1:5734,82=[::1]:5735,83=0.0.0.0:5736,443=192.168.1.10:8443")
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{
		80:  "host.docker.internal:49152",
		81:  "host.docker.internal:5734",
		82:  "host.docker.internal:5735",
		83:  "host.docker.internal:5736",
		443: "192.168.1.10:8443",
	}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("got %v, want %v", routes, want)
	}
}

func TestParseRoutesRejectsMalformedEntries(t *testing.T) {
	for _, spec := range []string{"", "80", "80=nohostport", "0=localhost:1", "x=localhost:1"} {
		if _, err := parseRoutes(spec); err == nil {
			t.Errorf("parseRoutes(%q): expected error", spec)
		}
	}
}
