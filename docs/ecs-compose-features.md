---
title: ECS integration Compose features
description: Reference list of compose ECS features
keywords: Docker, Amazon, Integration, ECS, Compose, cli, deploy, cloud
---
# Compose - Amazon ECS mapping

This document outlines the conversion of an application defined in a Compose file to AWS resources.
Each service is mapped to an ECS service in the project's cluster.

## Compose fields mapping

The table below lists supported Compose file fields and their AWS counterparts.

__Legend:__

- __✓:__ Implemented
- __n:__ Not yet implemented
- __x:__ Not applicable / no available conversion

| Keys                           |Map|  Notes                                                       |
|--------------------------------|---|--------------------------------------------------------------|
| __Service__                    | ✓ |
| service.service.build          | x |  Ignored. No image build support on AWS.
| service.cap_add, cap_drop      | ✓ |  Supported with [Fargate limitations](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_KernelCapabilities.html)
| service.command                | ✓ |
| service.configs                | x |
| service.cgroup_parent          | x |
| service.container_name         | x |
| service.credential_spec        | x |
| service.deploy                 | ✓ |
| service.deploy.endpoint_mode   | x |
| service.deploy.mode            | x |
| service.deploy.replicas        | ✓ |  Set service initial scale. Auto-scaling, when enabled, will make this dynamic
| service.deploy.placement       | ✓ |  Used with EC2 support to select a machine type and AMI
| service.deploy.update_config   | ✓ |
| service.deploy.resources       | ✓ |  Fargate resource is selected with the lowest instance type for configured memory and cpu
| service.deploy.restart_policy  | ✓ |
| service.deploy.labels          | ✓ |
| service.devices                | x |
| service.depends_on             | ✓ |  Implemented using CloudFormation Depends_on
| service.dns                    | x |
| service.dns_search             | x |
| service.domainname             | x |
| service.tmpfs                  | x |  Not supported on Fargate, see https://github.com/docker/compose-cli/issues/839
| service.entrypoint             | ✓ |
| service.env_file               | ✓ |
| service.environment            | ✓ |
| service.expose                 | x |
| service.extends                | ✓ |
| service.external_links         | x |
| service.extra_hosts            | x |
| service.group_add              | x |
| service.healthcheck            | ✓ |  This configures container level health check as reported on ECS console. Application Load Balancer will also check for HTTP service health by accessing `/` and expect a HTTP 200 status code.
| service.hostname               | x |
| service.image                  | ✓ |  Private images will be accessible by passing x-aws-pull_policy with ARN of a username+password secret
| service.isolation              | x |
| service.labels                 | x |
| service.links                  | x |
| service.logging                | ✓ |  Can be used to customize CloudWatch Logs configuration
| service.network_mode           | x |
| service.networks               | x |  Communication between services is implemented by SecurityGroups within the application VPC.
| service.pid                    | x |
| service.ports                  | ✓ |  Only symetrical port mapping is supported in ECS. See [Exposing ports](#exposing-ports).
| service.secrets                | ✓ |  See [Secrets](#secrets).
| service.security_opt           | x |
| service.stop_grace_period      | x |
| service.stop_signal            | x |
| service.sysctls                | x |
| service.ulimits                | ✓ |  Only support `nofile` ulimit due to Fargate limitations
| service.userns_mode            | x |
| service.volumes                | ✓ |  Mapped to EFS File Systems. See [Persistent volumes](#persistent-volumes).
| service.restart                | x |  Replaced by service.deployment.restart_policy
|                                |   |
| __Volume__                     | x |
| driver                         | ✓ |  See [Persistent volumes](#persistent-volumes).
| driver_opts                    | ✓ |
| external                       | ✓ |  `name` must be an EFS filesystem ID
| labels                         | x |
|                                |   |
| __Secret__                     | x |
| external                       | ✓ |  `name` must be set to secret's ARN
| file                           | ✓ |  file content will be uploaded into AWS Secret Manager
|                                |   |
| __Config__                     | x |
|                                |   |


## Logs

Application logs can be obtained container with `docker compose logs`.
The Docker ECS integration relies on AWS CloudWatch Logs to collect logs from all containers. CloudWatch can be customized by setting service `logging.driver_opts`
by passing configuration attributes prefixed with `awslogs-`.

```yaml
  test:
    image: mycompany/webapp
    logging:
      driver-opts:
        awslogs-datetime-pattern: "some-pattern"
```


## Exposing ports

When one or more services expose ports, a Load Balancer is created for the application.
As all services are exposed through the same Load Balancer, only one service can expose a given port number.
The source and target ports defined in the Compose file MUST be the same, as service-to-service communication don't go through the Load Balancer and could not
benefit from Listeners abstraction to assign a distinct published port.

If services in the Compose file only expose ports 80 or 443, an Application Load Balancer is created, otherwise ECS integration will provision a Network Load Balancer.
HTTP services using distinct ports can force use of an ALB by claiming the http protocol with `x-aws-protocol` custom extension within the port declaration:

```yaml
  test:
    image: mycompany/webapp
    ports:
      - target: 8080
        x-aws-protocol: http

```

## Persistent volumes

Docker volumes are mapped to EFS file systems. Volumes can be external (`name` must then be set to filesystem ID) or will be created when the application is
first deployed. `docker compose down` will NOT delete the filesystem, and it will be re-attached to the application on future runs.
`driver_opts` can be used to tweak the EFS filsystem.

Volume mount can be customized to workaround Posix filesystem permission issues by setting user and group IDs to be used to write to filesystem, whatever user
is configured to run the container.

```yaml
services:
    myservice:
        image: mycompany/webapp
        volumes:
        - mydata:/mount/testvolumes

volumes:
  mydata:
    driver_opts:
      performance-mode: maxIO
      throughput-mode: bursting
      uid: 0
      gid: 0
```


## Secrets

Secrets can be defined in compose files, and will need secret files available at deploy time next to the compose file.
The content of the secret file will be made available inside selected containers, by default under `/run/secrets/<SECRET_NAME>`.
External secrets are also supported, `name` must then be set to secret's ARN

```yaml
services:
    nginx:
        image: mycompany/webapp
        secrets:
          - mysecret

secrets:
  mysecret:
    file: ./my_secret1.txt
```


## Container Resources

CPU and memory limits can be set in compose. Those are used to select the minimal [Fargate size](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AWS_Fargate.html) that will match those limits.

```yaml
services:
    nginx:
        image: mycompany/webapp
        deploy:
          resources:
            limits:
              cpu: '0.5'
              memory: 2Gb
```
