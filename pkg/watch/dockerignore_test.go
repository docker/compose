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
