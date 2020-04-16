package compose

type API interface {
	ComposeUp(project *Project) error
	ComposeDown(project *Project) error
}