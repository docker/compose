package compose

type API interface {
	ComposeUp(project *Project, loadBalancerArn *string) error
	ComposeDown(project *Project, keepLoadBalancer bool) error
}