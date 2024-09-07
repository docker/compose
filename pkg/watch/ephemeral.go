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

// EphemeralPathMatcher filters out spurious changes that we don't want to
// rebuild on, like IDE temp/lock files.
//
// This isn't an ideal solution. In an ideal world, the user would put
// everything to ignore in their tiltignore/dockerignore files. This is a
// stop-gap so they don't have a terrible experience if those files aren't
// there or aren't in the right places.
//
// NOTE: The underlying `patternmatcher` is NOT always Goroutine-safe, so
// this is not a singleton; we create an instance for each watcher currently.
func EphemeralPathMatcher() PathMatcher {
	golandPatterns := []string{"**/*___jb_old___", "**/*___jb_tmp___", "**/.idea/**"}
	emacsPatterns := []string{"**/.#*", "**/#*#"}
	// if .swp is taken (presumably because multiple vims are running in that dir),
	// vim will go with .swo, .swn, etc, and then even .svz, .svy!
	// https://github.com/vim/vim/blob/ea781459b9617aa47335061fcc78403495260315/src/memline.c#L5076
	// ignoring .sw? seems dangerous, since things like .swf or .swi exist, but ignoring the first few
	// seems safe and should catch most cases
	vimPatterns := []string{"**/4913", "**/*~", "**/.*.swp", "**/.*.swx", "**/.*.swo", "**/.*.swn"}
	// kate (the default text editor for KDE) uses a file similar to Vim's .swp
	// files, but it doesn't have the "incrementing" character problem mentioned
	// above
	katePatterns := []string{"**/.*.kate-swp"}
	// go stdlib creates tmpfiles to determine umask for setting permissions
	// during file creation; they are then immediately deleted
	// https://github.com/golang/go/blob/0b5218cf4e3e5c17344ea113af346e8e0836f6c4/src/cmd/go/internal/work/exec.go#L1764
	goPatterns := []string{"**/*-go-tmp-umask"}

	var allPatterns []string
	allPatterns = append(allPatterns, golandPatterns...)
	allPatterns = append(allPatterns, emacsPatterns...)
	allPatterns = append(allPatterns, vimPatterns...)
	allPatterns = append(allPatterns, katePatterns...)
	allPatterns = append(allPatterns, goPatterns...)

	matcher, err := NewDockerPatternMatcher("/", allPatterns)
	if err != nil {
		panic(err)
	}
	return matcher
}
