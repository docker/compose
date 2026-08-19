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

package compose

import (
	"net/url"
	"strings"

	"github.com/docker/compose/v5/pkg/api"
)

// hookLogLine is a details-decorated log line split into its parts. The
// engine formats such lines as "k=v,k2=v2 payload" — attributes sorted,
// url-encoded, comma-joined, then a single space and the payload; the
// attribute block is empty for messages carrying no attributes.
type hookLogLine struct {
	// Hook is the hook type stamped by compose on captured hook execs
	// (post_start, pre_stop); empty for regular container output.
	Hook    string
	Payload string
}

// parseDetailsLine splits a details-decorated log line and extracts the
// compose hook identity, if any. timestamps indicates the line starts with
// an RFC3339 timestamp (the engine puts it before the attribute block);
// it is re-attached to the payload so display is unchanged.
func parseDetailsLine(line string, timestamps bool) hookLogLine {
	ts := ""
	if timestamps {
		if i := strings.IndexByte(line, ' '); i >= 0 {
			ts, line = line[:i+1], line[i+1:]
		}
	}
	attrs, payload, ok := strings.Cut(line, " ")
	if !ok {
		return hookLogLine{Payload: ts + line}
	}
	l := hookLogLine{Payload: ts + payload}
	for _, kv := range strings.Split(attrs, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k != api.HookLabel {
			continue
		}
		if hook, err := url.QueryUnescape(v); err == nil {
			l.Hook = hook
		}
	}
	return l
}
