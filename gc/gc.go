// Package gc experiments with providing central gc tooling to ensure
// deterministic resource removal within containerd.
//
// For now, we just have a single exported implementation that can be used
// under certain use cases.
package gc

// Tricolor implements basic, single-thread tri-color GC. Given the roots, the
// complete set and a refs function, this returns the unreachable objects.
//
// Correct usage requires that the caller not allow the arguments to change
// until the result is used to delete objects in the system.
//
// It will allocate memory proportional to the size of the reachable set.
//
// We can probably use this to inform a design for incremental GC by injecting
// callbacks to the set modification algorithms.
func Tricolor(roots []string, all []string, refs func(ref string) []string) []string {
	var (
		grays     []string                // maintain a gray "stack"
		seen      = map[string]struct{}{} // or not "white", basically "seen"
		reachable = map[string]struct{}{} // or "block", in tri-color parlance
	)

	grays = append(grays, roots...)

	for len(grays) > 0 {
		// Pick any gray object
		id := grays[len(grays)-1] // effectively "depth first" because first element
		grays = grays[:len(grays)-1]
		seen[id] = struct{}{} // post-mark this as not-white

		// mark all the referenced objects as gray
		for _, target := range refs(id) {
			if _, ok := seen[target]; !ok {
				grays = append(grays, target)
			}
		}

		// mark as black when done
		reachable[id] = struct{}{}
	}

	// All black objects are now reachable, and all white objects are
	// unreachable. Free those that are white!
	var whites []string
	for _, obj := range all {
		if _, ok := reachable[obj]; !ok {
			whites = append(whites, obj)
		}
	}

	return whites
}
