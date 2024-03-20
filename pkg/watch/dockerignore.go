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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/compose/v2/internal/paths"
	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

type dockerPathMatcher struct {
	repoRoot string
	matcher  *patternmatcher.PatternMatcher
}

func (i dockerPathMatcher) Matches(f string) (bool, error) {
	if !filepath.IsAbs(f) {
		f = filepath.Join(i.repoRoot, f)
	}
	return i.matcher.MatchesOrParentMatches(f)
}

func (i dockerPathMatcher) MatchesEntireDir(f string) (bool, error) {
	matches, err := i.Matches(f)
	if !matches || err != nil {
		return matches, err
	}

	// We match the dir, but we might exclude files underneath it.
	if i.matcher.Exclusions() {
		for _, pattern := range i.matcher.Patterns() {
			if !pattern.Exclusion() {
				continue
			}
			if paths.IsChild(f, pattern.String()) {
				// Found an exclusion match -- we don't match this whole dir
				return false, nil
			}
		}
		return true, nil
	}
	return true, nil
}

func LoadDockerIgnore(repoRoot string) (*dockerPathMatcher, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}

	patterns, err := readDockerignorePatterns(absRoot)
	if err != nil {
		return nil, err
	}

	return NewDockerPatternMatcher(absRoot, patterns)
}

// Make all the patterns use absolute paths.
func absPatterns(absRoot string, patterns []string) []string {
	absPatterns := make([]string, 0, len(patterns))
	for _, p := range patterns {
		// The pattern parsing here is loosely adapted from fileutils' NewPatternMatcher
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)

		pPath := p
		isExclusion := false
		if p[0] == '!' {
			pPath = p[1:]
			isExclusion = true
		}

		if !filepath.IsAbs(pPath) {
			pPath = filepath.Join(absRoot, pPath)
		}
		absPattern := pPath
		if isExclusion {
			absPattern = fmt.Sprintf("!%s", pPath)
		}
		absPatterns = append(absPatterns, absPattern)
	}
	return absPatterns
}

func NewDockerPatternMatcher(repoRoot string, patterns []string) (*dockerPathMatcher, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}

	pm, err := patternmatcher.New(absPatterns(absRoot, patterns))
	if err != nil {
		return nil, err
	}

	return &dockerPathMatcher{
		repoRoot: absRoot,
		matcher:  pm,
	}, nil
}

func readDockerignorePatterns(repoRoot string) ([]string, error) {
	var excludes []string

	f, err := os.Open(filepath.Join(repoRoot, ".dockerignore"))
	switch {
	case os.IsNotExist(err):
		return excludes, nil
	case err != nil:
		return nil, err
	}
	defer func() { _ = f.Close() }()

	patterns, err := ignorefile.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("error reading .dockerignore: %w", err)
	}
	return patterns, nil
}

func DockerIgnoreTesterFromContents(repoRoot string, contents string) (*dockerPathMatcher, error) {
	patterns, err := ignorefile.ReadAll(strings.NewReader(contents))
	if err != nil {
		return nil, fmt.Errorf("error reading .dockerignore: %w", err)
	}

	return NewDockerPatternMatcher(repoRoot, patterns)
}
