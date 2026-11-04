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

package watch

import (
	"testing"
)

func TestNewDockerPatternMatcher(t *testing.T) {
	tests := []struct {
		name         string
		repoRoot     string
		patterns     []string
		expectedErr  bool
		expectedRoot string
		expectedPat  []string
	}{
		{
			name:         "Basic patterns without wildcard",
			repoRoot:     "/repo",
			patterns:     []string{"dir1/", "file.txt"},
			expectedErr:  false,
			expectedRoot: "/repo",
			expectedPat:  []string{"/repo/dir1", "/repo/file.txt"},
		},
		{
			name:         "Patterns with exclusion",
			repoRoot:     "/repo",
			patterns:     []string{"dir1/", "!file.txt"},
			expectedErr:  false,
			expectedRoot: "/repo",
			expectedPat:  []string{"/repo/dir1", "!/repo/file.txt"},
		},
		{
			name:         "Wildcard with exclusion",
			repoRoot:     "/repo",
			patterns:     []string{"*", "!file.txt"},
			expectedErr:  false,
			expectedRoot: "/repo",
			expectedPat:  []string{"/repo/*", "!/repo/file.txt"},
		},
		{
			name:         "No patterns",
			repoRoot:     "/repo",
			patterns:     []string{},
			expectedErr:  false,
			expectedRoot: "/repo",
			expectedPat:  nil,
		},
		{
			name:         "Only exclusion pattern",
			repoRoot:     "/repo",
			patterns:     []string{"!file.txt"},
			expectedErr:  false,
			expectedRoot: "/repo",
			expectedPat:  []string{"!/repo/file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function with the test data
			matcher, err := NewDockerPatternMatcher(tt.repoRoot, tt.patterns)

			// Check if we expect an error
			if (err != nil) != tt.expectedErr {
				t.Fatalf("expected error: %v, got: %v", tt.expectedErr, err)
			}

			// If no error is expected, check the output
			if !tt.expectedErr {
				if matcher.repoRoot != tt.expectedRoot {
					t.Errorf("expected root: %v, got: %v", tt.expectedRoot, matcher.repoRoot)
				}

				// Compare patterns
				actualPatterns := matcher.matcher.Patterns()
				if len(actualPatterns) != len(tt.expectedPat) {
					t.Errorf("expected patterns length: %v, got: %v", len(tt.expectedPat), len(actualPatterns))
				}

				for i, expectedPat := range tt.expectedPat {
					actualPatternStr := actualPatterns[i].String()
					if actualPatterns[i].Exclusion() {
						actualPatternStr = "!" + actualPatternStr
					}
					if actualPatternStr != expectedPat {
						t.Errorf("expected pattern: %v, got: %v", expectedPat, actualPatterns[i])
					}
				}
			}
		})
	}
}

func TestDockerPathMatcher_MatchesAllowList(t *testing.T) {
	// Allow-list form: ignore everything, then re-include a subtree. The
	// parsed patterns alone don't prove this works -- only Matches does.
	tests := []struct {
		name    string
		content string
		path    string
		want    bool
	}{
		{
			name:    "allow-list ignores unrelated dir",
			content: "*\n!src\n",
			path:    "data/log",
			want:    true,
		},
		{
			name:    "allow-list ignores dotfile at root",
			content: "*\n!src\n",
			path:    ".env",
			want:    true,
		},
		{
			name:    "allow-list ignores nested dir",
			content: "*\n!src\n",
			path:    "node_modules/foo/index.js",
			want:    true,
		},
		{
			name:    "allow-list keeps re-included dir",
			content: "*\n!src\n",
			path:    "src/index.js",
			want:    false,
		},
		{
			name:    "allow-list keeps re-included dir itself",
			content: "*\n!src\n",
			path:    "src",
			want:    false,
		},
		{
			name:    "wildcard with single exclusion keeps excluded file",
			content: "*\n!file.txt\n",
			path:    "file.txt",
			want:    false,
		},
		{
			name:    "wildcard with single exclusion ignores other files",
			content: "*\n!file.txt\n",
			path:    "other.txt",
			want:    true,
		},
		{
			name:    "deny-list is unchanged",
			content: "data/\n",
			path:    "data/log",
			want:    true,
		},
		{
			name:    "deny-list keeps unrelated files",
			content: "data/\n",
			path:    "src/index.js",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := DockerIgnoreTesterFromContents("/repo", tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := matcher.Matches(tt.path)
			if err != nil {
				t.Fatalf("unexpected error matching %q: %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("Matches(%q) with .dockerignore %q = %v, want %v",
					tt.path, tt.content, got, tt.want)
			}
		})
	}
}
