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
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
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
			expectedPat:  []string{"!/repo/file.txt"},
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

func TestDockerPatternMatcherMatchesEntireDirWithWildcardExclusion(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src")
	for _, tc := range []struct {
		name      string
		exclusion string
		ignored   bool
	}{
		{name: "recursive wildcard", exclusion: "!**/app.txt"},
		{name: "single directory wildcard", exclusion: "!*/app.txt"},
		{name: "unrelated wildcard", exclusion: "!other/*.txt", ignored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := NewDockerPatternMatcher(root, []string{"src/", tc.exclusion})
			assert.NilError(t, err)

			ignored, err := matcher.MatchesEntireDir(dir)
			assert.NilError(t, err)
			assert.Equal(t, ignored, tc.ignored)

			ignored, err = matcher.Matches(filepath.Join(dir, "app.txt"))
			assert.NilError(t, err)
			assert.Equal(t, ignored, tc.ignored)
		})
	}
}
