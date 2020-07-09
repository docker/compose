# Docker CLI plugin for Amazon ECS

## Status

:exclamation: The Docker ECS plugin is still in Beta. It's design and UX will evolve until 1.0 Final release.

## Example

You can find an application for testing this in [example](./example).

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

## Application model

### Services

Compose services are mapped to ECS services. Compose specification has no support for multi-container services, nor 
does it support sidecars. When an ECS feature requires a sidecar, we introduce custom Compose extension (`x-aws-*`)
to actually expose ECS feature as a service-level feature, not plumbing details.

### Networking

We map the "network" abstraction from Compose model to AWS SecurityGroups. The whole application is created within a 
single VPC, SecurityGroups are created per networks, including the implicit `default` one. Services are attached 
according to the networks declared in Compose model. Doing so, services attached to a common security group can 
communicate together, while services from distinct SecurityGroups can't. We just can't set service aliasses per network.

A CloudMap private namespace is created for application as `{project}.local`. Services get registered so that we 
get service discovery and DNS round-robin (equivalent for Compose's `endpoint_mode: dnsrr`). Docker images SHOULD 
include a tiny entrypoint script to replicate this feature:
```shell script
if [ ! -z LOCALDOMAIN ]; then echo "search ${LOCALDOMAIN}" >> /etc/resolv.conf; fi 
```  

