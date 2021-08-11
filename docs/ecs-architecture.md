---
title: ECS integration architecture
description: Mapping of Docker compose entities to Amazon constructs
keywords: Docker, Amazon, Integration, ECS, Compose, architecture, mapping
---
# Architecture

ECS integration relies on CloudFormation to manage AWS resources as an atomic operation.
This document describes the mapping between compose application model and AWS components

## Overview

This diagram shows compose model and on same line AWS components that get created as equivalent resources

```
+----------+                                +-------------+                              +-------------------+
| Project  |  . . . . . . . . . . . . . .   | Cluster     |    . . . . . . .             | LoadBalancer      |
+-+--------+                                +-------------+                              +-------------------+
  |
  |    +----------+                         +-------------++-------------------+         +-------------------+
  +----+ Service  |   . . . . . . . . . .   | Service     || TaskDefinition    |         | TargetGroup       |
  |    +--+-------+                         +-------------++-------------------+-+       +-------------------+
  |       |                                                  | TaskRole          |
  |       |                                                  +-------------------+-+
  |       |  x-aws-role, x-aws-policies     . . . . . . . .    | TaskExecutionRole |
  |       |                                                    +-------------------+
  |       |  +---------+
  |       +--+ Deploy  |
  |       |  +---------+                    +-------------------+
  |       |  x-aws-autoscaling  . . . . . . | ScalableTarget    |
  |       |                                 +-------------------+---+
  |       |                                     | ScalingPolicy     |
  |       |                                     +-------------------+-+
  |       |                                       | AutoScalingRole   |
  |       |                                       +-------------------+
  |       |
  |       |  +---------+                    +-------------+                              +-------------------+
  |       +--+ Ports   |   . . . . . . .    | IngressRule +-----+                        | Listener          |
  |       |  +---------+                    +-------------+     |                        +-------------------+
  |       |                                                     |
  |       |  +---------+                    +---------------+ +------------------+
  |       +--+ Secrets |   . . . . . . .    | InitContainer | |TaskExecutionRole |
  |       |  +---------+                    +---------------+ +------------+-----+
  |       |                                                     |          |
  |       |  +---------+                                        |          |
  |       +--+ Volumes |                                        |          |
  |       |  +---------+                                        |          |
  |       |                                                     |          |
  |       |  +---------------+                                  |          |         +-------------------+
  |       +--+ DeviceRequest |  . . . . . . . . . . . . .  . .  | . . . .  | . . .   | CapacityProvider  |
  |          +---------------+                                  |          |         +-------------------+--------+
  |                                                             |          |                | AutoscalingGroup    |
  |   +------------+                        +---------------+   |          |                +---------------------+
  +---+ Networks   |   . . . . . . . . .    | SecurityGroup +---+          |                | LaunchConfiguration |
  |   +------------+                        +---------------+              |                +---------------------+
  |                                                                        |
  |   +------------+                        +---------------+              |
  +---+ Secret     |   . . . . . . . . .    | Secret        +--------------+
      +------------+                        +---------------+
```

Each compose application service is mapped to an ECS `Service`. A `TaskDefinition` is created according to compose definition.
Actual mapping is constrained by both Cloud platform and Fargate limitations. Such a `TaskDefinition` is set with a single container,
according to the compose model which doesn't offer a syntax to support sidecar containers.

An IAM Role is created and configured as `TaskRole` to grant service access to additional AWS resources when required. For this
purpose, user can set `x-aws-policies` or define a fine grained `x-aws-role` IAM role document.

Service's ports get mapped into security group's `IngressRule`s and load balancer `Listener`s.
Compose application with HTTP services only (using ports 80/443 or `x-aws-protocol` set to `http`) get an Application Load Balancer
created, otherwise a Network Load Balancer is used.

A `TargetGroup` is created per service to dispatch traffic by load balancer to the matching containers

Secrets bound to a service get translated into an `InitContainer` added to the service's `TaskDefinition`. This init container is
responsible to create a `/run/secrets` file for secret to match docker secret model and make application code portable.
A `TaskExecutionRole` is also created per service, and is updated to grant access to bound secrets.

Services using a GPU (`DeviceRequest`) get the `Cluster` extended with an EC2 `CapacityProvider`, using an `AutoscalingGroup` to manage
EC2 resources allocation based on a `LaunchConfiguration`. The latter uses ECS recommended AMI and machine type for GPU.

Service to declare `deploy.x-aws-autoscaling` get a `ScalingPolicy` created targeting specified the configured CPU usage metric
