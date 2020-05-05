# Docker CLI plugin for Amazon ECS

## Architecture

ECS plugin is a [Docker CLI plugin](https://docs.docker.com/engine/extend/cli_plugins/)
root command `ecs` require aws profile to get API credentials from `~/.aws/credentials`
as well as AWS region - those will later be stored in a docker context

A `compose.yaml` is parsed and converted into a [CloudFormation](https://aws.amazon.com/cloudformation/)
template, which will create all resources in dependent order and cleanup on
`down` command or deployment failure.

```
  +--------------------------------------+
  | compose.yaml file                    |
  +--------------------------------------+
- Load
  +--------------------------------------+
  | compose Model                        |
  +--------------------------------------+
- Validate
  +--------------------------------------+
  | compose Model suitable for ECS       |
  +--------------------------------------+
- Convert
  +--------------------------------------+
  | CloudFormation Template              |
  +--------------------------------------+
- Apply
  +--------------+      +----------------+  
  | AWS API      |  or  | stack file     |
  +--------------+      +----------------+
```

* _Load_ phase relies on [compose-go](https://github.com/compose-spec/compose-go). Any generic code we write for this 
purpose should be proposed upstream.
* _Validate_ phase is responsible to inject sane ECS defaults into the compose-go model, and validate the `compose.yaml` 
file do not include unsupported features.
* _Convert_ produces a CloudFormation template to define all resources required to implement the application model on AWS.
* _Apply_ phase do apply the CloudFormation template, either by exporting to a stack file or to deploy on AWS.  

