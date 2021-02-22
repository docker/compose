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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/internal"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/aws/aws-sdk-go/service/efs/efsiface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type sdk struct {
	ECS      ecsiface.ECSAPI
	EC2      ec2iface.EC2API
	EFS      efsiface.EFSAPI
	ELB      elbv2iface.ELBV2API
	CW       cloudwatchlogsiface.CloudWatchLogsAPI
	IAM      iamiface.IAMAPI
	CF       cloudformationiface.CloudFormationAPI
	SM       secretsmanageriface.SecretsManagerAPI
	SSM      ssmiface.SSMAPI
	AG       autoscalingiface.AutoScalingAPI
	S3       s3iface.S3API
	uploader *s3manager.Uploader
}

// sdk implement API
var _ API = sdk{}

func newSDK(sess *session.Session) sdk {
	sess.Handlers.Build.PushBack(func(r *request.Request) {
		request.AddToUserAgent(r, internal.ECSUserAgentName+"/"+internal.Version)
	})
	return sdk{
		ECS:      ecs.New(sess),
		EC2:      ec2.New(sess),
		EFS:      efs.New(sess),
		ELB:      elbv2.New(sess),
		CW:       cloudwatchlogs.New(sess),
		IAM:      iam.New(sess),
		CF:       cloudformation.New(sess),
		SM:       secretsmanager.New(sess),
		SSM:      ssm.New(sess),
		AG:       autoscaling.New(sess),
		S3:       s3.New(sess),
		uploader: s3manager.NewUploader(sess),
	}
}

func (s sdk) CheckRequirements(ctx context.Context, region string) error {
	settings, err := s.ECS.ListAccountSettingsWithContext(ctx, &ecs.ListAccountSettingsInput{
		EffectiveSettings: aws.Bool(true),
		Name:              aws.String("serviceLongArnFormat"),
	})
	if err != nil {
		return err
	}
	serviceLongArnFormat := settings.Settings[0].Value
	if *serviceLongArnFormat != "enabled" {
		return fmt.Errorf("this tool requires the \"new ARN resource ID format\".\n"+
			"Check https://%s.console.aws.amazon.com/ecs/home#/settings\n"+
			"Learn more: https://aws.amazon.com/blogs/compute/migrating-your-amazon-ecs-deployment-to-the-new-arn-and-resource-id-format-2", region)
	}
	return nil
}

func (s sdk) ResolveCluster(ctx context.Context, nameOrArn string) (awsResource, error) {
	logrus.Debug("CheckRequirements if cluster was already created: ", nameOrArn)
	clusters, err := s.ECS.DescribeClustersWithContext(ctx, &ecs.DescribeClustersInput{
		Clusters: []*string{aws.String(nameOrArn)},
	})
	if err != nil {
		return nil, err
	}
	if len(clusters.Clusters) == 0 {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "cluster %q does not exist", nameOrArn)
	}
	it := clusters.Clusters[0]
	return existingAWSResource{
		arn: aws.StringValue(it.ClusterArn),
		id:  aws.StringValue(it.ClusterName),
	}, nil
}

func (s sdk) CreateCluster(ctx context.Context, name string) (string, error) {
	logrus.Debug("Create cluster ", name)
	response, err := s.ECS.CreateClusterWithContext(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(name)})
	if err != nil {
		return "", err
	}
	return *response.Cluster.Status, nil
}

func (s sdk) CheckVPC(ctx context.Context, vpcID string) error {
	logrus.Debug("CheckRequirements on VPC : ", vpcID)
	output, err := s.EC2.DescribeVpcAttributeWithContext(ctx, &ec2.DescribeVpcAttributeInput{
		VpcId:     aws.String(vpcID),
		Attribute: aws.String("enableDnsSupport"),
	})
	if err != nil {
		return err
	}
	if !*output.EnableDnsSupport.Value {
		return fmt.Errorf("VPC %q doesn't have DNS resolution enabled", vpcID)
	}
	return nil
}

func (s sdk) GetDefaultVPC(ctx context.Context) (string, error) {
	logrus.Debug("Retrieve default VPC")
	vpcs, err := s.EC2.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(vpcs.Vpcs) == 0 {
		return "", fmt.Errorf("account has no default VPC. Set VPC to deploy to using 'x-aws-vpc'")
	}
	return *vpcs.Vpcs[0].VpcId, nil
}

func (s sdk) GetSubNets(ctx context.Context, vpcID string) ([]awsResource, error) {
	logrus.Debug("Retrieve SubNets")
	var ids []awsResource
	var token *string
	for {
		subnets, err := s.EC2.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: []*string{aws.String(vpcID)},
				},
			},
			NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, subnet := range subnets.Subnets {
			ids = append(ids, existingAWSResource{
				arn: aws.StringValue(subnet.SubnetArn),
				id:  aws.StringValue(subnet.SubnetId),
			})
		}

		if subnets.NextToken == token {
			break
		}
		token = subnets.NextToken
	}
	return ids, nil
}

func (s sdk) IsPublicSubnet(ctx context.Context, subNetID string) (bool, error) {
	tables, err := s.EC2.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{aws.String(subNetID)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	if len(tables.RouteTables) == 0 {
		// If a subnet is not explicitly associated with any route table, it is implicitly associated with the main route table.
		// https://docs.aws.amazon.com/cli/latest/reference/ec2/describe-route-tables.html
		return true, nil
	}
	for _, routeTable := range tables.RouteTables {
		for _, route := range routeTable.Routes {
			if aws.StringValue(route.State) != "active" {
				continue
			}
			if strings.HasPrefix(aws.StringValue(route.GatewayId), "igw-") {
				// Connected to an internet Gateway
				return true, nil
			}
		}
	}
	return false, nil
}

func (s sdk) GetRoleArn(ctx context.Context, name string) (string, error) {
	role, err := s.IAM.GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: aws.String(name),
	})
	if err != nil {
		return "", err
	}
	return *role.Role.Arn, nil
}

func (s sdk) StackExists(ctx context.Context, name string) (bool, error) {
	stacks, err := s.CF.DescribeStacksWithContext(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		if strings.HasPrefix(err.Error(), fmt.Sprintf("ValidationError: Stack with ID %s does not exist", name)) {
			return false, nil
		}
		return false, nil
	}
	return len(stacks.Stacks) > 0, nil
}

type uploadedTemplateFunc func(body *string, url *string) (string, error)

const cloudformationBytesLimit = 51200

func (s sdk) withTemplate(ctx context.Context, name string, template []byte, region string, fn uploadedTemplateFunc) (string, error) {
	if len(template) < cloudformationBytesLimit {
		return fn(aws.String(string(template)), nil)
	}

	key, err := uuid.GenerateUUID()
	if err != nil {
		return "", err
	}
	bucket := "com.docker.compose." + key
	logrus.Debugf("Create s3 bucket %q to store cloudformation template", bucket)

	var configuration *s3.CreateBucketConfiguration
	if region != "us-east-1" {
		configuration = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(region),
		}
	}
	_, err = s.S3.CreateBucket(&s3.CreateBucketInput{
		Bucket:                    aws.String(bucket),
		CreateBucketConfiguration: configuration,
	})
	if err != nil {
		return "", err
	}

	upload, err := s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Key:         aws.String("template.yaml"),
		Body:        bytes.NewReader(template),
		Bucket:      aws.String(bucket),
		ContentType: aws.String("application/x-yaml"),
		Tagging:     aws.String(name),
	})

	if err != nil {
		return "", err
	}

	defer func() {
		_, err := s.S3.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
			Bucket:    aws.String(bucket),
			Key:       aws.String("template.yaml"),
			VersionId: upload.VersionID,
		})
		if err != nil {
			logrus.Warnf("Failed to remove S3 bucket: %s", err)
		}
		_, err = s.S3.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			logrus.Warnf("Failed to remove S3 bucket: %s", err)
		}
	}()

	return fn(nil, aws.String(upload.Location))
}

func (s sdk) CreateStack(ctx context.Context, name string, region string, template []byte) error {
	logrus.Debug("Create CloudFormation stack")

	stackID, err := s.withTemplate(ctx, name, template, region, func(body *string, url *string) (string, error) {
		stack, err := s.CF.CreateStackWithContext(ctx, &cloudformation.CreateStackInput{
			OnFailure:        aws.String("DELETE"),
			StackName:        aws.String(name),
			TemplateBody:     body,
			TemplateURL:      url,
			TimeoutInMinutes: nil,
			Capabilities: []*string{
				aws.String(cloudformation.CapabilityCapabilityIam),
			},
			Tags: []*cloudformation.Tag{
				{
					Key:   aws.String(compose.ProjectTag),
					Value: aws.String(name),
				},
			},
		})
		if err != nil {
			return "", err
		}
		return aws.StringValue(stack.StackId), nil
	})
	logrus.Debugf("Stack %s created", stackID)
	return err
}

func (s sdk) CreateChangeSet(ctx context.Context, name string, region string, template []byte) (string, error) {
	logrus.Debug("Create CloudFormation Changeset")
	update := fmt.Sprintf("Update%s", time.Now().Format("2006-01-02-15-04-05"))

	changeset, err := s.withTemplate(ctx, name, template, region, func(body *string, url *string) (string, error) {
		changeset, err := s.CF.CreateChangeSetWithContext(ctx, &cloudformation.CreateChangeSetInput{
			ChangeSetName: aws.String(update),
			ChangeSetType: aws.String(cloudformation.ChangeSetTypeUpdate),
			StackName:     aws.String(name),
			TemplateBody:  body,
			TemplateURL:   url,
			Capabilities: []*string{
				aws.String(cloudformation.CapabilityCapabilityIam),
			},
		})
		if err != nil {
			return "", err
		}
		return aws.StringValue(changeset.Id), err
	})
	if err != nil {
		return "", err
	}

	// we have to WaitUntilChangeSetCreateComplete even this in fail with error `ResourceNotReady`
	// so that we can invoke DescribeChangeSet to check status, and then we can know about the actual creation failure cause.
	s.CF.WaitUntilChangeSetCreateCompleteWithContext(ctx, &cloudformation.DescribeChangeSetInput{ // nolint:errcheck
		ChangeSetName: aws.String(changeset),
	})

	desc, err := s.CF.DescribeChangeSetWithContext(ctx, &cloudformation.DescribeChangeSetInput{
		ChangeSetName: aws.String(update),
		StackName:     aws.String(name),
	})
	if aws.StringValue(desc.Status) == "FAILED" {
		return changeset, fmt.Errorf(aws.StringValue(desc.StatusReason))
	}

	return changeset, err
}

func (s sdk) UpdateStack(ctx context.Context, changeset string) error {
	desc, err := s.CF.DescribeChangeSetWithContext(ctx, &cloudformation.DescribeChangeSetInput{
		ChangeSetName: aws.String(changeset),
	})
	if err != nil {
		return err
	}

	if strings.HasPrefix(aws.StringValue(desc.StatusReason), "The submitted information didn't contain changes.") {
		return nil
	}

	_, err = s.CF.ExecuteChangeSet(&cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String(changeset),
	})
	return err
}

const (
	stackCreate = iota
	stackUpdate
	stackDelete
)

func (s sdk) WaitStackComplete(ctx context.Context, name string, operation int) error {
	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	}
	switch operation {
	case stackCreate:
		return s.CF.WaitUntilStackCreateCompleteWithContext(ctx, input)
	case stackDelete:
		return s.CF.WaitUntilStackDeleteCompleteWithContext(ctx, input)
	default:
		return fmt.Errorf("internal error: unexpected stack operation %d", operation)
	}
}

func (s sdk) GetStackID(ctx context.Context, name string) (string, error) {
	stacks, err := s.CF.DescribeStacksWithContext(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		return "", err
	}
	return *stacks.Stacks[0].StackId, nil
}

func (s sdk) ListStacks(ctx context.Context) ([]compose.Stack, error) {
	params := cloudformation.DescribeStacksInput{}
	var token *string
	var stacks []compose.Stack
	for {
		response, err := s.CF.DescribeStacksWithContext(ctx, &params)
		if err != nil {
			return nil, err
		}
		for _, stack := range response.Stacks {
			for _, t := range stack.Tags {
				if *t.Key == compose.ProjectTag {
					status := compose.RUNNING
					switch aws.StringValue(stack.StackStatus) {
					case "CREATE_IN_PROGRESS":
						status = compose.STARTING
					case "DELETE_IN_PROGRESS":
						status = compose.REMOVING
					case "UPDATE_IN_PROGRESS":
						status = compose.UPDATING
					default:
					}
					stacks = append(stacks, compose.Stack{
						ID:     aws.StringValue(stack.StackId),
						Name:   aws.StringValue(stack.StackName),
						Status: status,
					})
					break
				}
			}
		}
		if token == response.NextToken {
			return stacks, nil
		}
		token = response.NextToken
	}
}

func (s sdk) GetStackClusterID(ctx context.Context, stack string) (string, error) {
	// Note: could use DescribeStackResource but we only can detect `does not exist` case by matching string error message
	var token *string
	for {
		response, err := s.CF.ListStackResourcesWithContext(ctx, &cloudformation.ListStackResourcesInput{
			StackName: aws.String(stack),
		})
		if err != nil {
			return "", err
		}
		for _, r := range response.StackResourceSummaries {
			if aws.StringValue(r.ResourceType) == "AWS::ECS::Cluster" {
				return aws.StringValue(r.PhysicalResourceId), nil
			}
		}
		if token == response.NextToken {
			break
		}
		token = response.NextToken
	}
	// stack is using user-provided cluster
	res, err := s.CF.GetTemplateSummaryWithContext(ctx, &cloudformation.GetTemplateSummaryInput{
		StackName: aws.String(stack),
	})
	if err != nil {
		return "", err
	}
	c := aws.StringValue(res.Metadata)
	var m templateMetadata
	err = json.Unmarshal([]byte(c), &m)
	if err != nil {
		return "", err
	}
	if m.Cluster == "" {
		return "", errors.Wrap(errdefs.ErrNotFound, "CloudFormation is missing cluster metadata")
	}

	return m.Cluster, nil
}

type templateMetadata struct {
	Cluster string `json:",omitempty"`
}

func (s sdk) GetServiceTaskDefinition(ctx context.Context, cluster string, serviceArns []string) (map[string]string, error) {
	defs := map[string]string{}

	svc := []*string{}
	for _, s := range serviceArns {
		svc = append(svc, aws.String(s))
	}
	for i := 0; i < len(svc); i += 10 {
		end := i + 10
		if end > len(svc) {
			end = len(svc)
		}
		chunk := svc[i:end]
		services, err := s.ECS.DescribeServicesWithContext(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(cluster),
			Services: chunk,
		})
		if err != nil {
			return nil, err
		}
		for _, s := range services.Services {
			defs[aws.StringValue(s.ServiceArn)] = aws.StringValue(s.TaskDefinition)
		}
	}
	return defs, nil
}

func (s sdk) ListStackServices(ctx context.Context, stack string) ([]string, error) {
	arns := []string{}
	var nextToken *string
	for {
		response, err := s.CF.ListStackResourcesWithContext(ctx, &cloudformation.ListStackResourcesInput{
			StackName: aws.String(stack),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range response.StackResourceSummaries {
			if aws.StringValue(r.ResourceType) == "AWS::ECS::Service" {
				if r.PhysicalResourceId != nil {
					arns = append(arns, aws.StringValue(r.PhysicalResourceId))
				}
			}
		}
		nextToken = response.NextToken
		if nextToken == nil {
			break
		}
	}
	return arns, nil
}

func (s sdk) GetServiceTasks(ctx context.Context, cluster string, service string, stopped bool) ([]*ecs.Task, error) {
	state := "RUNNING"
	if stopped {
		state = "STOPPED"
	}
	var token *string
	var tasks []*ecs.Task
	for {
		response, err := s.ECS.ListTasksWithContext(ctx, &ecs.ListTasksInput{
			Cluster:       aws.String(cluster),
			ServiceName:   aws.String(service),
			DesiredStatus: aws.String(state),
		})
		if err != nil {
			return nil, err
		}
		if len(response.TaskArns) > 0 {
			taskDescriptions, err := s.ECS.DescribeTasksWithContext(ctx, &ecs.DescribeTasksInput{
				Cluster: aws.String(cluster),
				Tasks:   response.TaskArns,
			})
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, taskDescriptions.Tasks...)
		}
		if token == response.NextToken {
			return tasks, nil
		}
		token = response.NextToken
	}
}

func (s sdk) GetTaskStoppedReason(ctx context.Context, cluster string, taskArn string) (string, error) {
	taskDescriptions, err := s.ECS.DescribeTasksWithContext(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   []*string{aws.String(taskArn)},
	})
	if err != nil {
		return "", err
	}
	if len(taskDescriptions.Tasks) == 0 {
		return "", nil
	}
	task := taskDescriptions.Tasks[0]
	return fmt.Sprintf(
		"%s: %s",
		aws.StringValue(task.StopCode),
		aws.StringValue(task.StoppedReason)), nil

}

func (s sdk) DescribeStackEvents(ctx context.Context, stackID string) ([]*cloudformation.StackEvent, error) {
	// Fixme implement Paginator on Events and return as a chan(events)
	events := []*cloudformation.StackEvent{}
	var nextToken *string
	for {
		resp, err := s.CF.DescribeStackEventsWithContext(ctx, &cloudformation.DescribeStackEventsInput{
			StackName: aws.String(stackID),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		events = append(events, resp.StackEvents...)
		if resp.NextToken == nil {
			return events, nil
		}
		nextToken = resp.NextToken
	}
}

func (s sdk) ListStackParameters(ctx context.Context, name string) (map[string]string, error) {
	st, err := s.CF.DescribeStacksWithContext(ctx, &cloudformation.DescribeStacksInput{
		NextToken: nil,
		StackName: aws.String(name),
	})
	if err != nil {
		return nil, err
	}
	parameters := map[string]string{}
	for _, parameter := range st.Stacks[0].Parameters {
		parameters[aws.StringValue(parameter.ParameterKey)] = aws.StringValue(parameter.ParameterValue)
	}
	return parameters, nil
}

type stackResource struct {
	LogicalID string
	Type      string
	ARN       string
	Status    string
}

type stackResourceFn func(r stackResource) error

type stackResources []stackResource

func (resources stackResources) apply(awsType string, fn stackResourceFn) error {
	var errs *multierror.Error
	for _, r := range resources {
		if r.Type == awsType {
			err := fn(r)
			if err != nil {
				errs = multierror.Append(err)
			}
		}
	}
	return errs.ErrorOrNil()
}

func (s sdk) ListStackResources(ctx context.Context, name string) (stackResources, error) {
	var token *string
	var resources stackResources
	for {
		response, err := s.CF.ListStackResourcesWithContext(ctx, &cloudformation.ListStackResourcesInput{
			StackName: aws.String(name),
		})
		if err != nil {
			return nil, err
		}

		for _, r := range response.StackResourceSummaries {
			resources = append(resources, stackResource{
				LogicalID: aws.StringValue(r.LogicalResourceId),
				Type:      aws.StringValue(r.ResourceType),
				ARN:       aws.StringValue(r.PhysicalResourceId),
				Status:    aws.StringValue(r.ResourceStatus),
			})
		}
		if token == response.NextToken {
			return resources, nil
		}
		token = response.NextToken
	}
}

func (s sdk) DeleteStack(ctx context.Context, name string) error {
	logrus.Debug("Delete CloudFormation stack")
	_, err := s.CF.DeleteStackWithContext(ctx, &cloudformation.DeleteStackInput{
		StackName: aws.String(name),
	})
	return err
}

func (s sdk) CreateSecret(ctx context.Context, secret secrets.Secret) (string, error) {
	logrus.Debug("Create secret " + secret.Name)
	var tags []*secretsmanager.Tag
	for k, v := range secret.Labels {
		tags = []*secretsmanager.Tag{
			{
				Key:   aws.String(k),
				Value: aws.String(v),
			},
		}
	}
	// store the secret content as string
	content := string(secret.GetContent())
	response, err := s.SM.CreateSecret(&secretsmanager.CreateSecretInput{
		Name:         &secret.Name,
		SecretString: &content,
		Tags:         tags,
	})
	if err != nil {
		return "", err
	}
	return aws.StringValue(response.ARN), nil
}

func (s sdk) InspectSecret(ctx context.Context, id string) (secrets.Secret, error) {
	logrus.Debug("Inspect secret " + id)
	response, err := s.SM.DescribeSecret(&secretsmanager.DescribeSecretInput{SecretId: &id})
	if err != nil {
		return secrets.Secret{}, err
	}
	tags := map[string]string{}
	for _, tag := range response.Tags {
		tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}

	secret := secrets.Secret{
		ID:     aws.StringValue(response.ARN),
		Name:   aws.StringValue(response.Name),
		Labels: tags,
	}
	return secret, nil
}

func (s sdk) ListSecrets(ctx context.Context) ([]secrets.Secret, error) {
	logrus.Debug("List secrets ...")
	var ls []secrets.Secret
	var token *string
	for {
		response, err := s.SM.ListSecrets(&secretsmanager.ListSecretsInput{})
		if err != nil {
			return nil, err
		}

		for _, sec := range response.SecretList {

			tags := map[string]string{}
			for _, tag := range sec.Tags {
				tags[*tag.Key] = *tag.Value
			}
			ls = append(ls, secrets.Secret{
				ID:     *sec.ARN,
				Name:   *sec.Name,
				Labels: tags,
			})
		}

		if token == response.NextToken {
			return ls, nil
		}
		token = response.NextToken
	}
}

func (s sdk) DeleteSecret(ctx context.Context, id string, recover bool) error {
	logrus.Debug("List secrets ...")
	force := !recover
	_, err := s.SM.DeleteSecret(&secretsmanager.DeleteSecretInput{SecretId: &id, ForceDeleteWithoutRecovery: &force})
	return err
}

func (s sdk) GetLogs(ctx context.Context, name string, consumer func(name string, service string, container string, message string), follow bool) error {
	logGroup := fmt.Sprintf("/docker-compose/%s", name)
	var startTime int64
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var hasMore = true
			var token *string
			for hasMore {
				events, err := s.CW.FilterLogEvents(&cloudwatchlogs.FilterLogEventsInput{
					LogGroupName: aws.String(logGroup),
					NextToken:    token,
					StartTime:    aws.Int64(startTime),
				})
				if err != nil {
					return err
				}
				if events.NextToken == nil {
					hasMore = false
				} else {
					token = events.NextToken
				}

				for _, event := range events.Events {
					p := strings.Split(aws.StringValue(event.LogStreamName), "/")
					consumer(p[1], p[1], p[2], aws.StringValue(event.Message))
					startTime = *event.IngestionTime
				}
			}
		}
		if !follow {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s sdk) DescribeService(ctx context.Context, cluster string, arn string) (compose.ServiceStatus, error) {
	services, err := s.ECS.DescribeServicesWithContext(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(cluster),
		Services: []*string{aws.String(arn)},
		Include:  aws.StringSlice([]string{"TAGS"}),
	})
	if err != nil {
		return compose.ServiceStatus{}, err
	}

	for _, f := range services.Failures {
		return compose.ServiceStatus{}, errors.Wrapf(errdefs.ErrNotFound, "can't get service status %s: %s", aws.StringValue(f.Detail), aws.StringValue(f.Reason))
	}
	service := services.Services[0]
	var name string
	for _, t := range service.Tags {
		if *t.Key == compose.ServiceTag {
			name = aws.StringValue(t.Value)
		}
	}
	if name == "" {
		return compose.ServiceStatus{}, fmt.Errorf("service %s doesn't have a %s tag", *service.ServiceArn, compose.ServiceTag)
	}
	targetGroupArns := []string{}
	for _, lb := range service.LoadBalancers {
		targetGroupArns = append(targetGroupArns, *lb.TargetGroupArn)
	}
	// getURLwithPortMapping makes 2 queries
	// one to get the target groups and another for load balancers
	loadBalancers, err := s.getURLWithPortMapping(ctx, targetGroupArns)
	if err != nil {
		return compose.ServiceStatus{}, err
	}
	return compose.ServiceStatus{
		ID:         aws.StringValue(service.ServiceName),
		Name:       name,
		Replicas:   int(aws.Int64Value(service.RunningCount)),
		Desired:    int(aws.Int64Value(service.DesiredCount)),
		Publishers: loadBalancers,
	}, nil
}

func (s sdk) DescribeServiceTasks(ctx context.Context, cluster string, project string, service string) ([]compose.ContainerSummary, error) {
	var summary []compose.ContainerSummary
	familly := fmt.Sprintf("%s-%s", project, service)
	var token *string
	for {
		list, err := s.ECS.ListTasks(&ecs.ListTasksInput{
			Cluster:    aws.String(cluster),
			Family:     aws.String(familly),
			LaunchType: nil,
			MaxResults: nil,
			NextToken:  token,
		})
		if err != nil {
			return nil, err
		}

		if len(list.TaskArns) == 0 {
			break
		}
		tasks, err := s.ECS.DescribeTasksWithContext(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Include: aws.StringSlice([]string{"TAGS"}),
			Tasks:   list.TaskArns,
		})
		if err != nil {
			return nil, err
		}

		for _, t := range tasks.Tasks {
			var project string
			var service string
			for _, tag := range t.Tags {
				switch aws.StringValue(tag.Key) {
				case compose.ProjectTag:
					project = aws.StringValue(tag.Value)
				case compose.ServiceTag:
					service = aws.StringValue(tag.Value)
				}
			}

			id, err := arn.Parse(aws.StringValue(t.TaskArn))
			if err != nil {
				return nil, err
			}

			summary = append(summary, compose.ContainerSummary{
				ID:      id.String(),
				Name:    id.Resource,
				Project: project,
				Service: service,
				State:   strings.Title(strings.ToLower(aws.StringValue(t.LastStatus))),
			})
		}

		if list.NextToken == token {
			break
		}
		token = list.NextToken
	}

	return summary, nil
}

func (s sdk) getURLWithPortMapping(ctx context.Context, targetGroupArns []string) ([]compose.PortPublisher, error) {
	if len(targetGroupArns) == 0 {
		return nil, nil
	}
	groups, err := s.ELB.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: aws.StringSlice(targetGroupArns),
	})
	if err != nil {
		return nil, err
	}
	lbarns := []*string{}
	for _, tg := range groups.TargetGroups {
		lbarns = append(lbarns, tg.LoadBalancerArns...)
	}

	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: lbarns,
	})

	if err != nil {
		return nil, err
	}
	filterLB := func(arn *string, lbs []*elbv2.LoadBalancer) *elbv2.LoadBalancer {
		if aws.StringValue(arn) == "" {
			// load balancer arn is nil/""
			return nil
		}
		for _, lb := range lbs {
			if aws.StringValue(lb.LoadBalancerArn) == aws.StringValue(arn) {
				return lb
			}
		}
		return nil
	}
	loadBalancers := []compose.PortPublisher{}
	for _, tg := range groups.TargetGroups {
		for _, lbarn := range tg.LoadBalancerArns {
			lb := filterLB(lbarn, lbs.LoadBalancers)
			if lb == nil {
				continue
			}
			loadBalancers = append(loadBalancers, compose.PortPublisher{
				URL:           fmt.Sprintf("%s:%d", aws.StringValue(lb.DNSName), aws.Int64Value(tg.Port)),
				TargetPort:    int(aws.Int64Value(tg.Port)),
				PublishedPort: int(aws.Int64Value(tg.Port)),
				Protocol:      strings.ToLower(aws.StringValue(tg.Protocol)),
			})

		}
	}
	return loadBalancers, nil
}

func (s sdk) ListTasks(ctx context.Context, cluster string, family string) ([]string, error) {
	var token *string
	var arns []string
	for {
		response, err := s.ECS.ListTasksWithContext(ctx, &ecs.ListTasksInput{
			Cluster: aws.String(cluster),
			Family:  aws.String(family),
		})
		if err != nil {
			return nil, err
		}
		for _, arn := range response.TaskArns {
			arns = append(arns, *arn)
		}
		if token == response.NextToken {
			return arns, nil
		}
		token = response.NextToken
	}
}

func (s sdk) GetPublicIPs(ctx context.Context, interfaces ...string) (map[string]string, error) {
	var token *string
	publicIPs := map[string]string{}
	for {
		response, err := s.EC2.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: aws.StringSlice(interfaces),
		})
		if err != nil {
			return nil, err
		}
		for _, interf := range response.NetworkInterfaces {
			if interf.Association != nil {
				publicIPs[aws.StringValue(interf.NetworkInterfaceId)] = aws.StringValue(interf.Association.PublicIp)
			}
		}
		if token == response.NextToken {
			return publicIPs, nil
		}
		token = response.NextToken
	}
}

func (s sdk) ResolveLoadBalancer(ctx context.Context, nameOrArn string) (awsResource, string, string, []awsResource, error) {
	logrus.Debug("Check if LoadBalancer exists: ", nameOrArn)
	var arns []*string
	var names []*string
	if arn.IsARN(nameOrArn) {
		arns = append(arns, aws.String(nameOrArn))
	} else {
		names = append(names, aws.String(nameOrArn))
	}

	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: arns,
		Names:            names,
	})
	if err != nil {
		return nil, "", "", nil, err
	}
	if len(lbs.LoadBalancers) == 0 {
		return nil, "", "", nil, errors.Wrapf(errdefs.ErrNotFound, "load balancer %q does not exist", nameOrArn)
	}
	it := lbs.LoadBalancers[0]
	var subNets []awsResource
	for _, az := range it.AvailabilityZones {
		subNets = append(subNets, existingAWSResource{
			id: aws.StringValue(az.SubnetId),
		})
	}
	return existingAWSResource{
		arn: aws.StringValue(it.LoadBalancerArn),
		id:  aws.StringValue(it.LoadBalancerName),
	}, aws.StringValue(it.Type), aws.StringValue(it.VpcId), subNets, nil
}

func (s sdk) GetLoadBalancerURL(ctx context.Context, arn string) (string, error) {
	logrus.Debug("Retrieve load balancer URL: ", arn)
	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return "", err
	}
	dnsName := aws.StringValue(lbs.LoadBalancers[0].DNSName)
	if dnsName == "" {
		return "", fmt.Errorf("Load balancer %s doesn't have a dns name", aws.StringValue(lbs.LoadBalancers[0].LoadBalancerArn))
	}
	return dnsName, nil
}

func (s sdk) GetParameter(ctx context.Context, name string) (string, error) {
	parameter, err := s.SSM.GetParameterWithContext(ctx, &ssm.GetParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return "", err
	}

	value := *parameter.Parameter.Value
	var ami struct {
		SchemaVersion     int    `json:"schema_version"`
		ImageName         string `json:"image_name"`
		ImageID           string `json:"image_id"`
		OS                string `json:"os"`
		ECSRuntimeVersion string `json:"ecs_runtime_verion"`
		ECSAgentVersion   string `json:"ecs_agent_version"`
	}
	err = json.Unmarshal([]byte(value), &ami)
	if err != nil {
		return "", err
	}

	return ami.ImageID, nil
}

func (s sdk) SecurityGroupExists(ctx context.Context, sg string) (bool, error) {
	desc, err := s.EC2.DescribeSecurityGroupsWithContext(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: aws.StringSlice([]string{sg}),
	})
	if err != nil {
		return false, err
	}
	return len(desc.SecurityGroups) > 0, nil
}

func (s sdk) DeleteCapacityProvider(ctx context.Context, arn string) error {
	_, err := s.ECS.DeleteCapacityProvider(&ecs.DeleteCapacityProviderInput{
		CapacityProvider: aws.String(arn),
	})
	return err
}

func (s sdk) DeleteAutoscalingGroup(ctx context.Context, arn string) error {
	_, err := s.AG.DeleteAutoScalingGroup(&autoscaling.DeleteAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(arn),
		ForceDelete:          aws.Bool(true),
	})
	return err
}

func (s sdk) ResolveFileSystem(ctx context.Context, id string) (awsResource, error) {
	desc, err := s.EFS.DescribeFileSystemsWithContext(ctx, &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(id),
	})
	if err != nil {
		return nil, err
	}
	if len(desc.FileSystems) == 0 {
		return nil, errors.Wrapf(errdefs.ErrNotFound, "EFS file system %q doesn't exist", id)
	}
	it := desc.FileSystems[0]
	return existingAWSResource{
		arn: aws.StringValue(it.FileSystemArn),
		id:  aws.StringValue(it.FileSystemId),
	}, nil
}

func (s sdk) ListFileSystems(ctx context.Context, tags map[string]string) ([]awsResource, error) {
	var results []awsResource
	var token *string
	for {
		desc, err := s.EFS.DescribeFileSystemsWithContext(ctx, &efs.DescribeFileSystemsInput{
			Marker: token,
		})
		if err != nil {
			return nil, err
		}
		for _, filesystem := range desc.FileSystems {
			if containsAll(filesystem.Tags, tags) {
				results = append(results, existingAWSResource{
					arn: aws.StringValue(filesystem.FileSystemArn),
					id:  aws.StringValue(filesystem.FileSystemId),
				})
			}
		}
		if desc.NextMarker == token {
			return results, nil
		}
		token = desc.NextMarker
	}
}

func containsAll(tags []*efs.Tag, required map[string]string) bool {
TAGS:
	for key, value := range required {
		for _, t := range tags {
			if aws.StringValue(t.Key) == key && aws.StringValue(t.Value) == value {
				continue TAGS
			}
		}
		return false
	}
	return true
}

func (s sdk) CreateFileSystem(ctx context.Context, tags map[string]string, options VolumeCreateOptions) (awsResource, error) {
	var efsTags []*efs.Tag
	for k, v := range tags {
		efsTags = append(efsTags, &efs.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	var (
		k *string
		p *string
		f *float64
		t *string
	)
	if options.ProvisionedThroughputInMibps > 1 {
		f = aws.Float64(options.ProvisionedThroughputInMibps)
	}
	if options.KmsKeyID != "" {
		k = aws.String(options.KmsKeyID)
	}
	if options.PerformanceMode != "" {
		p = aws.String(options.PerformanceMode)
	}
	if options.ThroughputMode != "" {
		t = aws.String(options.ThroughputMode)
	}
	res, err := s.EFS.CreateFileSystemWithContext(ctx, &efs.CreateFileSystemInput{
		Encrypted:                    aws.Bool(true),
		KmsKeyId:                     k,
		PerformanceMode:              p,
		ProvisionedThroughputInMibps: f,
		ThroughputMode:               t,
		Tags:                         efsTags,
	})
	if err != nil {
		return nil, err
	}
	return existingAWSResource{
		id:  aws.StringValue(res.FileSystemId),
		arn: aws.StringValue(res.FileSystemArn),
	}, nil
}

func (s sdk) DeleteFileSystem(ctx context.Context, id string) error {
	_, err := s.EFS.DeleteFileSystemWithContext(ctx, &efs.DeleteFileSystemInput{
		FileSystemId: aws.String(id),
	})
	return err
}
