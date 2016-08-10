package containerd

import "fmt"

// VersionMajor holds the release major number
const VersionMajor = 0

// VersionMinor holds the release minor number
const VersionMinor = 2

// VersionPatch holds the release patch number
const VersionPatch = 3

// Version holds the combination of major minor and patch as a string
// of format Major.Minor.Patch
var Version = fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)

// GitCommit is filled with the Git revision being used to build the
// program at linking time
var GitCommit = ""
