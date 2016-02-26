package runtime

// Checkpoint is not supported on Windows.
// TODO Windows: Can eventually be factored out entirely.
type Checkpoint struct {
}

// PlatformProcessState container platform-specific fields in the ProcessState structure
type PlatformProcessState struct {
}
