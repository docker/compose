package ignore

import (
	"github.com/tilt-dev/tilt/internal/dockerignore"
	"github.com/tilt-dev/tilt/pkg/model"
)

// EphemeralPathMatcher filters out spurious changes that we don't want to
// rebuild on, like IDE temp/lock files.
//
// This isn't an ideal solution. In an ideal world, the user would put
// everything to ignore in their tiltignore/dockerignore files. This is a
// stop-gap so they don't have a terrible experience if those files aren't
// there or aren't in the right places.
//
// https://app.clubhouse.io/windmill/story/691/filter-out-ephemeral-file-changes
var EphemeralPathMatcher = initEphemeralPathMatcher()

func initEphemeralPathMatcher() model.PathMatcher {
	golandPatterns := []string{"**/*___jb_old___", "**/*___jb_tmp___", "**/.idea/**"}
	emacsPatterns := []string{"**/.#*", "**/#*#"}
	// if .swp is taken (presumably because multiple vims are running in that dir),
	// vim will go with .swo, .swn, etc, and then even .svz, .svy!
	// https://github.com/vim/vim/blob/ea781459b9617aa47335061fcc78403495260315/src/memline.c#L5076
	// ignoring .sw? seems dangerous, since things like .swf or .swi exist, but ignoring the first few
	// seems safe and should catch most cases
	vimPatterns := []string{"**/4913", "**/*~", "**/.*.swp", "**/.*.swx", "**/.*.swo", "**/.*.swn"}
	// kate (the default text editor for KDE) uses a file similar to Vim's .swp
	// files, but it doesn't have the "incrememnting" character problem mentioned
	// above
	katePatterns := []string{"**/.*.kate-swp"}
	// go stdlib creates tmpfiles to determine umask for setting permissions
	// during file creation; they are then immediately deleted
	// https://github.com/golang/go/blob/0b5218cf4e3e5c17344ea113af346e8e0836f6c4/src/cmd/go/internal/work/exec.go#L1764
	goPatterns := []string{"**/*-go-tmp-umask"}

	allPatterns := []string{}
	allPatterns = append(allPatterns, golandPatterns...)
	allPatterns = append(allPatterns, emacsPatterns...)
	allPatterns = append(allPatterns, vimPatterns...)
	allPatterns = append(allPatterns, katePatterns...)
	allPatterns = append(allPatterns, goPatterns...)

	matcher, err := dockerignore.NewDockerPatternMatcher("/", allPatterns)
	if err != nil {
		panic(err)
	}
	return matcher
}
