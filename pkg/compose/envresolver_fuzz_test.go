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
	"strings"
	"testing"
)

// FuzzEnvResolverWithCase locks the resolution semantics: the resolver is
// exactly the map when case-sensitive, and an exact match must win over the
// lower-cased fallback when case-insensitive.
func FuzzEnvResolverWithCase(f *testing.F) {
	f.Add("FOO", "bar", "BAZ", "qux", "foo")
	f.Add("PATH", "/bin", "HOME", "/root", "path")
	f.Fuzz(func(t *testing.T, k1, v1, k2, v2, query string) {
		// keep the two keys distinct after case folding so the lower-cased
		// fallback has a single unambiguous winner
		if strings.EqualFold(k1, k2) {
			k2 += "#"
		}
		environment := map[string]string{k1: v1, k2: v2}

		// nil environment never resolves
		if v, ok := envResolverWithCase(nil, true)(query); ok || v != "" {
			t.Fatalf("nil environment must never resolve %q, got (%q, %t)", query, v, ok)
		}

		// case-sensitive: exactly the map
		gotV, gotOK := envResolverWithCase(environment, false)(query)
		if wantOK := query == k1 || query == k2; gotOK != wantOK || gotV != environment[query] {
			t.Fatalf("case-sensitive resolver disagrees with the map for %q: got (%q, %t), want (%q, %t)",
				query, gotV, gotOK, environment[query], wantOK)
		}

		// case-insensitive: exact match first, then lower-cased fallback
		gotV, gotOK = envResolverWithCase(environment, true)(query)
		wantV, wantOK := "", false
		if query == k1 || query == k2 {
			wantV, wantOK = environment[query], true
		} else {
			switch strings.ToLower(query) {
			case strings.ToLower(k1):
				wantV, wantOK = v1, true
			case strings.ToLower(k2):
				wantV, wantOK = v2, true
			}
		}
		if gotV != wantV || gotOK != wantOK {
			t.Fatalf("case-insensitive resolver for %q: got (%q, %t), want (%q, %t)",
				query, gotV, gotOK, wantV, wantOK)
		}
	})
}
