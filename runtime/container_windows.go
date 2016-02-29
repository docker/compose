package runtime

import "errors"

func getRootIDs(s *PlatformSpec) (int, int, error) {
	return 0, 0, nil
}

func (c *container) Runtime() string {
	return "windows"
}

func (c *container) Pause() error {
	return errors.New("Pause not supported on Windows")
}

func (c *container) Resume() error {
	return errors.New("Resume not supported on Windows")
}

func (c *container) Checkpoints() ([]Checkpoint, error) {
	return nil, errors.New("Checkpoints not supported on Windows ")
}

func (c *container) Checkpoint(cpt Checkpoint) error {
	return errors.New("Checkpoint not supported on Windows ")
}

func (c *container) DeleteCheckpoint(name string) error {
	return errors.New("DeleteCheckpoint not supported on Windows ")
}

// TODO Windows: Implement me.
// This will have a very different implementation on Windows.
func (c *container) Start(checkpoint string, s Stdio) (Process, error) {
	return nil, errors.New("Start not yet implemented on Windows")
}

// TODO Windows: Implement me.
// This will have a very different implementation on Windows.
func (c *container) Exec(pid string, spec ProcessSpec, s Stdio) (Process, error) {
	return nil, errors.New("Exec not yet implemented on Windows")
}

// TODO Windows: Implement me.
func (c *container) Pids() ([]int, error) {
	return nil, errors.New("Pids not yet implemented on Windows")
}

// TODO Windows: Implement me. (Not yet supported by docker on Windows either...)
func (c *container) Stats() (*Stat, error) {
	return nil, errors.New("Stats not yet implemented on Windows")
}
