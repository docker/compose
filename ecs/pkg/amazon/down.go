package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/sirupsen/logrus"
)

func (c *client) ComposeDown(project *compose.Project, keepLoadBalancer bool) error {
	err := c.DeleteLoadBalancer(project, keepLoadBalancer)
	if err != nil {
		return err
	}

	services := []*string{}
	// FIXME we should be able to retrieve services by tags, so we don't need the initial compose file to run "down"
	for _, service := range project.Services {
		logrus.Debugf("Deleting service %q\n", service.Name)
		out, err := c.ECS.DeleteService(&ecs.DeleteServiceInput{
			// Force to true so that we don't have to scale down to 0
			// before deleting
			Force:   aws.Bool(true),
			Cluster: aws.String(c.Cluster),
			Service: aws.String(service.Name),
		})
		if err != nil {
			return err
		}
		services = append(services, out.Service.ServiceArn)
	}

	logrus.Info("Stopping services...")
	err = c.ECS.WaitUntilServicesInactive(&ecs.DescribeServicesInput{
		Services: services,
	})
	if err != nil {
		return err
	}
	logrus.Info("All services stopped")

	logrus.Debug("Deleting security groups")
	groups, err := c.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + ProjectTag),
				Values: aws.StringSlice([]string{project.Name}),
			},
		},
	})
	if err != nil {
		return err
	}
	for _, g := range groups.SecurityGroups {
		_, err = c.EC2.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
			GroupId: g.GroupId,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
