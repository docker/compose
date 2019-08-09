package ignore

import (
	"github.com/windmilleng/tilt/internal/dockerignore"
	"github.com/windmilleng/tilt/pkg/model"
)

// Filter out spurious changes that we don't want to rebuild on, like IDE
// temp/lock files.
//
// This isn't an ideal solution. In an ideal world, the user would put
// everything to ignore in their tiltignore/dockerignore files. This is a
// stop-gap so they don't have a terrible experience if those files aren't
// there or aren't in the right places.
//
// https://app.clubhouse.io/windmill/story/691/filter-out-ephemeral-file-changes
var ephemeralPathMatcher = initEphemeralPathMatcher()

func initEphemeralPathMatcher() model.PathMatcher {
	golandPatterns := []string{"**/*___jb_old___", "**/*___jb_tmp___"}
	emacsPatterns := []string{"**/.#*"}
	vimPatterns := []string{"**/4913", "**/*~", "**/.*.swp", "**/.*.swx"}

	allPatterns := []string{}
	allPatterns = append(allPatterns, golandPatterns...)
	allPatterns = append(allPatterns, emacsPatterns...)
	allPatterns = append(allPatterns, vimPatterns...)

	matcher, err := dockerignore.NewDockerPatternMatcher("/", allPatterns)
	if err != nil {
		panic(err)
	}
	return matcher
}
