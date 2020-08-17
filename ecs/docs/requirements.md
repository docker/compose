## Requirements

This plugin relies on AWS API credentials, using the same configuration files as
the AWS command line.

Such credentials can be configured by the `docker ecs setup` command, either by 
selecting an existing AWS CLI profile from existing config files, or by creating
one passing an AWS access key ID and secret access key.

## Permissions

AWS accounts (or IAM roles) used with the ECS plugin require following permissions:

- ec2:DescribeSubnets  
- ec2:DescribeVpcs
- iam:CreateServiceLinkedRole
- iam:AttachRolePolicy
- cloudformation:*
- ecs:*
- logs:*
- servicediscovery:*
- elasticloadbalancing:*


## Okta support

For those relying on [aws-okta](https://github.com/segmentio/aws-okta) to access a managed AWS account 
(as we do at Docker), you can populate your aws config files with temporary access tokens using: 
```shell script
aws-okta write-to-credentials <profile> ~/.aws/credentials
```
