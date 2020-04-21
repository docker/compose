package compose

import "github.com/awslabs/goformation/v4/cloudformation"

type API interface {
	Convert(project *Project, loadBalancerArn *string) (*cloudformation.Template, error)
	ComposeUp(project *Project, loadBalancerArn *string) error
	ComposeDown(project *Project, keepLoadBalancer bool) error
}
