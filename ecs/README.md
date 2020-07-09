# Docker CLI plugin for Amazon ECS

This was announced at AWS Cloud Containers Conference 2020, read the
[blog post](https://www.docker.com/blog/from-docker-straight-to-aws/).

## Status

:exclamation: The Docker ECS plugin is still in Beta.
Its design and UX will evolve until 1.0 Final release.

## Get started

If you're using macOS or Windows you just need to install
[Docker Desktop Edge](https://www.docker.com/products/docker-desktop) and you
will have the ECS integration installed.

You can find Linux install instructions [here](./docs/get-started-linux.md).

## Example and documentation

You can find an application for testing this in [example](./example).

You can find more documentation about using the Docker ECS integration
[here](https://docs.docker.com/engine/context/ecs-integration/).

## Architecture

The Docker ECS integration is a
[Docker CLI plugin](https://docs.docker.com/engine/extend/cli_plugins/)
with the root command of `ecs`.
It requires an AWS profile to select the AWS API credentials from
`~/.aws/credentials` as well as an AWS region. You can read more about CLI AWS
credential management
[here](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html).
Once setup, the AWS profile and region are stored in a Docker context.

A `compose.yaml` file is parsed and converted into a
[CloudFormation](https://aws.amazon.com/cloudformation/) template,
which is then used to create all application resources in dependent order.
Resources are cleaned up with the `down` command or in the case of a deployment
failure.

The architecture of the ECS integration is shown below:

```
  +--------------------------------------+
  | compose.yaml file                    |
  +--------------------------------------+
- Load
  +--------------------------------------+
  | Compose Model                        |
  +--------------------------------------+
- Validate
  +--------------------------------------+
  | Compose Model suitable for ECS       |
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

* The _Load_ phase relies on
  [compose-go](https://github.com/compose-spec/compose-go).
  Any generic code we write for this purpose should be proposed upstream.
* The _Validate_ phase is responsible for injecting sane ECS defaults into the
  compose-go model, and validating the `compose.yaml` file does not include
  unsupported features.
* The _Convert_ phase produces a CloudFormation template to define all
  application resources required to implement the application model on AWS.
* The _Apply_ phase does the actual apply of the CloudFormation template,
  either by exporting to a stack file or to deploy on AWS.

## Application model

### Services

Compose services are mapped to ECS services. The Compose specification does not
have support for multi-container services (like Kubernetes pods) or sidecars.
When an ECS feature requires a sidecar, we use a custom Compose extension
(`x-aws-*`) to expose the ECS features as a service-level feature,
and keep the plumbing details from the user.

### Networking

We map the "network" abstraction from the Compose model to AWS security groups.
The whole application is created within a single VPC,
security groups are created per Compose network, including the implicit
`default` one.
Services are attached according to the networks declared in Compose model.
This approach means that services attached to a common security group can
communicate together, while services from distinct security groups cannot.
This matches the intent of the Compose network model with the limitation that we
cannot set service aliases per network.

A [CloudMap](https://aws.amazon.com/cloud-map/) private namespace is created for
each application as `{project}.local`. Services get registered so that we
have service discovery and DNS round-robin
(equivalent to Compose's `endpoint_mode: dnsrr`). Docker images SHOULD include
an entrypoint script to replicate this feature:

```shell script
if [ ! -z LOCALDOMAIN ]; then echo "search ${LOCALDOMAIN}" >> /etc/resolv.conf; fi
```
