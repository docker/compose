package backend

const (
	ECSTaskExecutionPolicy = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
	ECRReadOnlyPolicy      = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"

	ActionGetSecretValue = "secretsmanager:GetSecretValue"
	ActionGetParameters  = "ssm:GetParameters"
	ActionDecrypt        = "kms:Decrypt"
)

var assumeRolePolicyDocument = PolicyDocument{
	Version: "2012-10-17", // https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_version.html
	Statement: []PolicyStatement{
		{
			Effect: "Allow",
			Principal: PolicyPrincipal{
				Service: "ecs-tasks.amazonaws.com",
			},
			Action: []string{"sts:AssumeRole"},
		},
	},
}

// could alternatively depend on https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/cmd/clusterawsadm/api/iam/v1alpha1/types.go
type PolicyDocument struct {
	Version   string            `json:",omitempty"`
	Statement []PolicyStatement `json:",omitempty"`
}

type PolicyStatement struct {
	Effect    string          `json:",omitempty"`
	Action    []string        `json:",omitempty"`
	Principal PolicyPrincipal `json:",omitempty"`
	Resource  []string        `json:",omitempty"`
}

type PolicyPrincipal struct {
	Service string `json:",omitempty"`
}
