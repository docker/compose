---
title: ECS integration composefile examples
description: Examples of ECS compose files
keywords: Docker, Amazon, Integration, ECS, Compose, cli, deploy, cloud, sample
---
# Compose file samples - ECS specific



## Service

A service mapping may define a Docker image and runtime constraints and container requirements.

```yaml
services:
  test:
    image: "image"
    command: "command"
    entrypoint: "entrypoint"
    environment:
      - "FOO=BAR"
    cap_add:
      - SYS_PTRACE
    cap_drop:
      - SYSLOG
    init: true
    user: "user"
    working_dir: "working_dir"
```


###### Task size

Set resource limits that will get translated to Fargate task size values:

```yaml
services:
  test:
    image: nginx
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 2048M
```

###### IAM roles

Assign an existing user role to a task:

```yaml
services:
  test:
    x-aws-policies:
      - "arn:aws:iam::aws:policy/AmazonS3FullAccess"
```

###### IAM policies

Assign an in-line IAM policy to a task:

```yaml
services:
  test:
    x-aws-role:
        Version: '2012-10-17'
        Statement:
        - Effect: Allow
          Action: sqs:*
          Resource: arn:aws:sqs:us-east-1:12345678:myqueue
```

###### Logging
Pass options to awslogs driver
```yaml
services:
  foo:
    image: nginx
    logging:
      options:
        awslogs-datetime-pattern: "FOO"

x-aws-logs_retention: 10
```


###### Autoscaling

Set a CPU percent target
```yaml
services:
  foo:
    image: nginx
    deploy:
      x-aws-autoscaling: 
        cpu: 75
```


###### GPU
Set `generic_resources` for services that require accelerators as GPUs.
```yaml
services:
  learning:
    image: tensorflow/tensorflow:latest-gpus
    deploy:
      resources:
        reservations:
          memory: 32Gb
          cpus: "32"
          generic_resources:
            - discrete_resource_spec:
                kind: gpus
                value: 2
```




##### Load Balancers

When a service in the compose file exposes a port, a load balancer is being created and configured to distribute the traffic between all containers.

There are 2 types of Load Balancers that can be created. For a service exposing a non-http port/protocol, a __Network Load Balancer (NLB)__ is created. Services with http/https ports/protocols get an __Application Load Balancer (ALB)__.

 There is only one load balancer created/configured for a Compose stack. If there are both http/non-http ports configured for services in a compose stack, an NLB is created.

The compose file below configured only the http port,therefore, on deployment it gets an ALB created.

```yaml
services:
  app:
    image: nginx
    ports:
      - 80:80
```
NLB is created for non-http port
```yaml
services:
  app:
    image: nginx
    ports:
      - 8080:8080
```

To use the http protocol with custom ports and get an ALB, use the `x-aws-protocol` port property.
```yaml
services:
  test:
    image: nginx
    ports:
      - target: 8080
        x-aws-protocol: http
```

To re-use an external load balancer and avoid creating a dedicated one, set the top-level property `x-aws-loadbalancer` as below:
```yaml
x-aws-loadbalancer: "LoadBalancerName"
services:
  app:
    image: nginx
    ports:
      - 80:80
```

Similarly, an external `VPC` and `Cluster` can be reused:

```yaml
x-aws-vpc: "vpc-25435e"
x-aws-cluster: "ClusterName"

services:
  app:
    image: nginx
    ports:
      - 80:80
```

Keep in mind, that external resources are not managed as part of the compose stack's lifecycle.


## Volumes

```yaml
services:
  app:
    image: nginx
    volumes:
      - data:/test
volumes:
  data:
```
To use of an external volume that has been previously created, set its id/ARN as the name:

```yaml
services:
  app:
    image: nginx
    volumes:
      - data:/test

volumes:
  data:
    external: true
    name: "fs-f534645"
```

Customize volume configuration via `driver_opts`

```yaml
services:
  test:
    image: nginx
volumes:
  db-data:
    driver_opts:
        backup_policy: ENABLED
        lifecycle_policy: AFTER_30_DAYS
        performance_mode: maxIO
        throughput_mode: provisioned
        provisioned_throughput: 1024
```

## Networks

Networks are mapped to security groups.
```yaml
services:
  test:
    image: nginx
networks:
  default:
```
Using an external network/security group:
```yaml
services:
  test:
    image: nginx
networks:
  default:
    external: true
    name: sg-123abc
```

## Secrets
Secrets are stored in __AWS SecretsManager__ as strings and are mounted to containers  under `/run/secrets/`.
```yaml
services:
  app:
    image: nginx
    ports:
      - 80:80
    secrets:
      - mysecret

secrets:
  mysecret:
    file: ./secrets/mysecret.txt
```

When using external secrets, set a valid secret `ARN` under the `name` property:

```yaml
services:
  app:
    image: nginx
    secrets:
      - foo_bar

secrets:
  foo_bar:
    name: "arn:aws:secretsmanager:eu-west-3:xxx:secret:foo_bar"
    external: true
```


## Access private images
When a service is configured with an image from a private repository on Docker Hub, make sure you have configured pull credentials correctly before deploying the Compose stack.

To create a pull credential, create a file with the following content:
```sh
$ cat creds.json
{
  "username":"DockerHubID",
  "password":"GeneratedHubTokenOrPassword"
}
```
To create the pull credential and retrieve the `ARN/ID` to use in the compose file run:
```sh
$ docker secret create pullcred /path/to/creds.json
arn:aws:secretsmanager:eu-west-3:xxx:secret:pullcred
```

Use the `ARN` in the output to set the `x-aws-pull_credentials` service property as below:
```yaml
services:
  app:
    image: DockerHubID/privateimage
    x-aws-pull_credentials: arn:aws:secretsmanager:eu-west-3:xxx:secret:pullcred
    ports:
      - 80:80
```
