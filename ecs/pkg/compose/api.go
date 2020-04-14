package compose

type API interface {
	ComposeUp(project *Project) error
}