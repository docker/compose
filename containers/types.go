package containers

const (
	// RestartPolicyAny Always restarts
	RestartPolicyAny = "any"
	// RestartPolicyNone Never restarts
	// "no" is the value for docker run, "none" is the value in compose file (and default differ
	RestartPolicyNone = "none"
	// RestartPolicyOnFailure Restarts only on failure
	RestartPolicyOnFailure = "on-failure"
)
