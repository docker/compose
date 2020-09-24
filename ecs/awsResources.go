/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package ecs

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/elasticloadbalancingv2"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
)

// awsResources hold the AWS component being used or created to support services definition
type awsResources struct {
	sdk              sdk
	vpc              string
	subnets          []string
	cluster          string
	loadBalancer     string
	loadBalancerType string
	securityGroups   map[string]string
}

func (r *awsResources) serviceSecurityGroups(service types.ServiceConfig) []string {
	var groups []string
	for net := range service.Networks {
		groups = append(groups, r.securityGroups[net])
	}
	return groups
}

func (r *awsResources) allSecurityGroups() []string {
	var securityGroups []string
	for _, r := range r.securityGroups {
		securityGroups = append(securityGroups, r)
	}
	return securityGroups
}

// parse look into compose project for configured resource to use, and check they are valid
func (r *awsResources) parse(ctx context.Context, project *types.Project) error {
	return findProjectFnError(ctx, project,
		r.parseClusterExtension,
		r.parseVPCExtension,
		r.parseLoadBalancerExtension,
		r.parseSecurityGroupExtension,
	)
}

func (r *awsResources) parseClusterExtension(ctx context.Context, project *types.Project) error {
	if x, ok := project.Extensions[extensionCluster]; ok {
		cluster := x.(string)
		ok, err := r.sdk.ClusterExists(ctx, cluster)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cluster does not exist: %s", cluster)
		}
		r.cluster = cluster
	}
	return nil
}

func (r *awsResources) parseVPCExtension(ctx context.Context, project *types.Project) error {
	if x, ok := project.Extensions[extensionVPC]; ok {
		vpc := x.(string)
		err := r.sdk.CheckVPC(ctx, vpc)
		if err != nil {
			return err
		}
		r.vpc = vpc
	} else {
		defaultVPC, err := r.sdk.GetDefaultVPC(ctx)
		if err != nil {
			return err
		}
		r.vpc = defaultVPC
	}

	subNets, err := r.sdk.GetSubNets(ctx, r.vpc)
	if err != nil {
		return err
	}
	if len(subNets) < 2 {
		return fmt.Errorf("VPC %s should have at least 2 associated subnets in different availability zones", r.vpc)
	}
	r.subnets = subNets
	return nil
}

func (r *awsResources) parseLoadBalancerExtension(ctx context.Context, project *types.Project) error {
	if x, ok := project.Extensions[extensionLoadBalancer]; ok {
		loadBalancer := x.(string)
		loadBalancerType, err := r.sdk.LoadBalancerType(ctx, loadBalancer)
		if err != nil {
			return err
		}

		required := getRequiredLoadBalancerType(project)
		if loadBalancerType != required {
			return fmt.Errorf("load balancer %s is of type %s, project require a %s", loadBalancer, loadBalancerType, required)
		}

		r.loadBalancer = loadBalancer
		r.loadBalancerType = loadBalancerType
	}
	return nil
}

func (r *awsResources) parseSecurityGroupExtension(ctx context.Context, project *types.Project) error {
	if r.securityGroups == nil {
		r.securityGroups = make(map[string]string, len(project.Networks))
	}
	for name, net := range project.Networks {
		if net.External.External {
			r.securityGroups[name] = net.Name
		}
		if x, ok := net.Extensions[extensionSecurityGroup]; ok {
			logrus.Warn("to use an existing security-group, use `network.external` and `network.name` in your compose file")
			logrus.Debugf("Security Group for network %q set by user to %q", net.Name, x)
			r.securityGroups[name] = x.(string)
		}
	}
	return nil
}

// ensure all required resources pre-exists or are defined as cloudformation resources
func (r *awsResources) ensure(project *types.Project, template *cloudformation.Template) {
	r.ensureCluster(project, template)
	r.ensureNetworks(project, template)
	r.ensureLoadBalancer(project, template)
}

func (r *awsResources) ensureCluster(project *types.Project, template *cloudformation.Template) {
	if r.cluster != "" {
		return
	}
	template.Resources["Cluster"] = &ecs.Cluster{
		ClusterName: project.Name,
		Tags:        projectTags(project),
	}
	r.cluster = cloudformation.Ref("Cluster")
}

func (r *awsResources) ensureNetworks(project *types.Project, template *cloudformation.Template) {
	if r.securityGroups == nil {
		r.securityGroups = make(map[string]string, len(project.Networks))
	}
	for name, net := range project.Networks {
		securityGroup := networkResourceName(name)
		template.Resources[securityGroup] = &ec2.SecurityGroup{
			GroupDescription: fmt.Sprintf("%s Security Group for %s network", project.Name, name),
			GroupName:        securityGroup,
			VpcId:            r.vpc,
			Tags:             networkTags(project, net),
		}

		ingress := securityGroup + "Ingress"
		template.Resources[ingress] = &ec2.SecurityGroupIngress{
			Description:           fmt.Sprintf("Allow communication within network %s", name),
			IpProtocol:            "-1", // all protocols
			GroupId:               cloudformation.Ref(securityGroup),
			SourceSecurityGroupId: cloudformation.Ref(securityGroup),
		}

		r.securityGroups[name] = cloudformation.Ref(securityGroup)
	}
}

func (r *awsResources) ensureLoadBalancer(project *types.Project, template *cloudformation.Template) {
	if r.loadBalancer != "" {
		return
	}
	if allServices(project.Services, func(it types.ServiceConfig) bool {
		return len(it.Ports) == 0
	}) {
		logrus.Debug("Application does not expose any public port, so no need for a LoadBalancer")
		return
	}

	balancerType := getRequiredLoadBalancerType(project)
	template.Resources["LoadBalancer"] = &elasticloadbalancingv2.LoadBalancer{
		Scheme:         elbv2.LoadBalancerSchemeEnumInternetFacing,
		SecurityGroups: r.getLoadBalancerSecurityGroups(project),
		Subnets:        r.subnets,
		Tags:           projectTags(project),
		Type:           balancerType,
	}
	r.loadBalancer = cloudformation.Ref("LoadBalancer")
	r.loadBalancerType = balancerType
}

func (r *awsResources) getLoadBalancerSecurityGroups(project *types.Project) []string {
	securityGroups := []string{}
	for name, network := range project.Networks {
		if !network.Internal {
			securityGroups = append(securityGroups, r.securityGroups[name])
		}
	}
	return securityGroups
}

func getRequiredLoadBalancerType(project *types.Project) string {
	loadBalancerType := elbv2.LoadBalancerTypeEnumNetwork
	if allServices(project.Services, func(it types.ServiceConfig) bool {
		return allPorts(it.Ports, portIsHTTP)
	}) {
		loadBalancerType = elbv2.LoadBalancerTypeEnumApplication
	}
	return loadBalancerType
}

func portIsHTTP(it types.ServicePortConfig) bool {
	if v, ok := it.Extensions[extensionProtocol]; ok {
		protocol := v.(string)
		return protocol == "http" || protocol == "https"
	}
	return it.Target == 80 || it.Target == 443
}

type projectFn func(ctx context.Context, project *types.Project) error

func findProjectFnError(ctx context.Context, project *types.Project, funcs ...projectFn) error {
	for _, fn := range funcs {
		err := fn(ctx, project)
		if err != nil {
			return err
		}
	}
	return nil
}

// predicate[types.ServiceConfig]
type servicePredicate func(it types.ServiceConfig) bool

// all[types.ServiceConfig]
func allServices(services types.Services, p servicePredicate) bool {
	for _, s := range services {
		if !p(s) {
			return false
		}
	}
	return true
}

// predicate[types.ServicePortConfig]
type portPredicate func(it types.ServicePortConfig) bool

// all[types.ServicePortConfig]
func allPorts(ports []types.ServicePortConfig, p portPredicate) bool {
	for _, s := range ports {
		if !p(s) {
			return false
		}
	}
	return true
}
