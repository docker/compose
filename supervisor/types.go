package supervisor

// State constants used in Event types
const (
	StateStart        = "start-container"
	StatePause        = "pause"
	StateResume       = "resume"
	StateExit         = "exit"
	StateStartProcess = "start-process"
	StateOOM          = "oom"
	StateLive         = "live"
)
