# Docker CLI plugin for Amazon ECS

## Architecture

ECS plugin is a [Docker CLI plugin](https://docs.docker.com/engine/extend/cli_plugins/)
root command `ecs` require aws profile to get API credentials from `~/.aws/credentials`
as well as AWS region - those will later be stored in a docker context

A `compose.yaml` is parsed and converted into a [CloudFormation](https://aws.amazon.com/cloudformation/)
template, which will create all resources in dependent order and cleanup on
`down` command or deployment failure.

```
  +-----------------------------+
  | compose.yaml file           |
  +-----------------------------+
- Load
  +-----------------------------+
  | compose-go Model            |
  +-----------------------------+
- Convert
  +-----------------------------+
  | CloudFormation Template     |
  +-----------------------------+
- Apply
  +---------+      +------------+  
  | AWS API |  or  | stack file |
  +---------+      +------------+
```

(if this sounds familiar, see [Kompose](https://github.com/kubernetes/kompose/blob/master/docs/architecture.md))