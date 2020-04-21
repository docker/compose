package amazon

import (
	"fmt"
	"strings"

	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/compose-spec/compose-go/types"
)

func (c client) CreateLoadBalancer(project *compose.Project, subnets []*string) (*string, error) {
	logrus.Debug("Create Load Balancer")
	alb, err := c.ELB.CreateLoadBalancer(&elbv2.CreateLoadBalancerInput{
		IpAddressType: nil,
		Name:          aws.String(fmt.Sprintf("%s-LoadBalancer", project.Name)),
		Subnets:       subnets,
		Type:          aws.String(elbv2.LoadBalancerTypeEnumNetwork),
		Tags: []*elbv2.Tag{
			{
				Key:   aws.String("com.docker.compose.project"),
				Value: aws.String(project.Name),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return alb.LoadBalancers[0].LoadBalancerArn, nil
}

func (c client) DeleteLoadBalancer(project *compose.Project, keepLoadBalancer bool) error {
	logrus.Debug("Delete Load Balancer")
	// FIXME We can tag LoadBalancer but not search by tag ?
	loadBalancer, err := c.ELB.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
		Names: aws.StringSlice([]string{fmt.Sprintf("%s-LoadBalancer", project.Name)}),
	})
	if err != nil {
		return err
	}
	arn := loadBalancer.LoadBalancers[0].LoadBalancerArn

	err = c.DeleteListeners(arn)
	if err != nil {
		return err
	}

	err = c.DeleteTargetGroups(arn)
	if err != nil {
		return err
	}

	if !keepLoadBalancer {
		_, err = c.ELB.DeleteLoadBalancer(&elbv2.DeleteLoadBalancerInput{LoadBalancerArn: arn})
	}
	return err
}

func (c client) CreateTargetGroup(name string, vpc *string, port types.ServicePortConfig) (*string, error) {
	logrus.Debugf("Create Target Group %d/%s\n", port.Target, port.Protocol)
	group, err := c.ELB.CreateTargetGroup(&elbv2.CreateTargetGroupInput{
		Name:       aws.String(name),
		Port:       aws.Int64(int64(port.Target)),
		Protocol:   aws.String(strings.ToUpper(port.Protocol)),
		TargetType: aws.String("ip"),
		VpcId:      vpc,
	})
	if err != nil {
		return nil, err
	}
	arn := group.TargetGroups[0].TargetGroupArn
	return arn, nil
}

func (c client) DeleteTargetGroups(loadBalancer *string) error {
	groups, err := c.ELB.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: loadBalancer,
	})
	if err != nil {
		return err
	}
	for _, group := range groups.TargetGroups {
		logrus.Debugf("Delete Target Group %s\n", *group.TargetGroupArn)
		_, err := c.ELB.DeleteTargetGroup(&elbv2.DeleteTargetGroupInput{
			TargetGroupArn: group.TargetGroupArn,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c client) CreateListener(port types.ServicePortConfig, arn *string, target *string) error {
	logrus.Debugf("Create Listener %d\n", port.Published)
	_, err := c.ELB.CreateListener(&elbv2.CreateListenerInput{
		DefaultActions: []*elbv2.Action{
			{
				ForwardConfig: &elbv2.ForwardActionConfig{
					TargetGroups: []*elbv2.TargetGroupTuple{
						{
							TargetGroupArn: target,
						},
					},
				},
				Type: aws.String(elbv2.ActionTypeEnumForward),
			},
		},
		LoadBalancerArn: arn,
		Port:            aws.Int64(int64(port.Published)),
		Protocol:        aws.String(strings.ToUpper(port.Protocol)),
	})
	return err
}

func (c client) DeleteListeners(loadBalancer *string) error {
	listeners, err := c.ELB.DescribeListeners(&elbv2.DescribeListenersInput{
		LoadBalancerArn: loadBalancer,
	})
	if err != nil {
		return err
	}
	for _, listener := range listeners.Listeners {
		logrus.Debugf("Delete Listener %s\n", *listener.ListenerArn)
		_, err := c.ELB.DeleteListener(&elbv2.DeleteListenerInput{
			ListenerArn: listener.ListenerArn,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
