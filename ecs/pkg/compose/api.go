package compose

import "github.com/awslabs/goformation/v4/cloudformation"

type API interface {
	Convert(project *Project) (*cloudformation.Template, error)
	ComposeUp(project *Project) error
	ComposeDown(projectName string, deleteCluster bool) error
}
