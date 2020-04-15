package amazon

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/sirupsen/logrus"
	"strings"
)

// GetDefaultVPC retrieve the default VPC for AWS account
func (c client) GetDefaultVPC() (*string, error) {
	logrus.Debug("Retrieve default VPC")
	vpcs, err := c.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(vpcs.Vpcs) == 0 {
		return nil, fmt.Errorf("account has not default VPC")
	}
	return vpcs.Vpcs[0].VpcId, nil
}


// GetSubNets retrieve default subnets for a VPC
func (c client) GetSubNets(vpc *string) ([]*string, error) {
	logrus.Debug("Retrieve SubNets")
	subnets, err := c.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		DryRun: nil,
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpc},
			},
			{
				Name:   aws.String("default-for-az"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	ids := []*string{}
	for _, subnet := range subnets.Subnets {
		ids = append(ids, subnet.SubnetId)
	}
	return ids, nil
}

// CreateSecurityGroup create a security group for the project
func (c client) CreateSecurityGroup(project *compose.Project, vpc *string) (*string, error) {
	logrus.Debug("Create Security Group")
	name := fmt.Sprintf("%s Security Group", project.Name)
	securityGroup, err := c.EC2.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		Description: aws.String(name),
		GroupName:   aws.String(name),
		VpcId:       vpc,
	})
	if err != nil {
		return nil, err
	}

	_, err = c.EC2.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{securityGroup.GroupId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(name),
			},
			{
				Key:   aws.String(ProjectTag),
				Value: aws.String(project.Name),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return securityGroup.GroupId, nil
}


func (c *client) ExposePort(securityGroup *string, port types.ServicePortConfig) error {
	logrus.Debugf("Authorize ingress port %d/%s\n", port.Published, port.Protocol)
	_, err := c.EC2.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: securityGroup,
		IpPermissions: []*ec2.IpPermission{
			{
				IpProtocol: aws.String(strings.ToUpper(port.Protocol)),
				IpRanges: []*ec2.IpRange{
					{
						CidrIp: aws.String("0.0.0.0/0"),
					},
				},
				FromPort: aws.Int64(int64(port.Target)),
				ToPort:   aws.Int64(int64(port.Target)),
			},
		},
	})
	return err
}