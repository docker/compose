package runtime

import "time"

type Checkpoint struct {
	// Timestamp is the time that checkpoint happened
	Created time.Time `json:"created"`
	// Name is the name of the checkpoint
	Name string `json:"name"`
	// Tcp checkpoints open tcp connections
	Tcp bool `json:"tcp"`
	// UnixSockets persists unix sockets in the checkpoint
	UnixSockets bool `json:"unixSockets"`
	// Shell persists tty sessions in the checkpoint
	Shell bool `json:"shell"`
	// Exit exits the container after the checkpoint is finished
	Exit bool `json:"exit"`
}

// PlatformProcessState container platform-specific fields in the ProcessState structure
type PlatformProcessState struct {
	Checkpoint string `json:"checkpoint"`
	RootUID    int    `json:"rootUID"`
	RootGID    int    `json:"rootGID"`
}
