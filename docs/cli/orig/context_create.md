---
title: "context create"
description: "The context create command description and usage"
keywords: "context, create"
---

<!-- This file is maintained within the docker/cli GitHub
     repository at https://github.com/docker/cli/. Make all
     pull requests against that repo. If you see this file in
     another repository, consider it read-only there, as it will
     periodically be overwritten by the definitive file. Pull
     requests which include edits to this file in other repositories
     will be rejected.
-->

# context create

```markdown
Usage:  docker context create [OPTIONS] CONTEXT

Create a context

Docker endpoint config:

NAME                DESCRIPTION
from                Copy Docker endpoint configuration from an existing context
host                Docker endpoint on which to connect
ca                  Trust certs signed only by this CA
cert                Path to TLS certificate file
key                 Path to TLS key file
skip-tls-verify     Skip TLS certificate validation

Kubernetes endpoint config:

NAME                 DESCRIPTION
from                 Copy Kubernetes endpoint configuration from an existing context
config-file          Path to a Kubernetes config file
context-override     Overrides the context set in the kubernetes config file
namespace-override   Overrides the namespace set in the kubernetes config file

Example:

$ docker context create my-context \
      --description "some description" \
      --docker "host=tcp://myserver:2376,ca=~/ca-file,cert=~/cert-file,key=~/key-file"

Options:
      --default-stack-orchestrator string   Default orchestrator for
                                            stack operations to use with
                                            this context
                                            (swarm|kubernetes|all)
      --description string                  Description of the context
      --docker stringToString               Set the docker endpoint
                                            (default [])
      --kubernetes stringToString           Set the kubernetes endpoint
                                            (default [])
      --from string                         Create the context from an existing context
```

## Description

Creates a new `context`. This allows you to quickly switch the cli
configuration to connect to different clusters or single nodes.

To create a context from scratch provide the docker and, if required,
kubernetes options. The example below creates the context `my-context`
with a docker endpoint of `/var/run/docker.sock` and a kubernetes configuration
sourced from the file `/home/me/my-kube-config`:

```bash
$ docker context create my-context \
      --docker host=/var/run/docker.sock \
      --kubernetes config-file=/home/me/my-kube-config
```

Use the `--from=<context-name>` option to create a new context from
an existing context. The example below creates a new context named `my-context`
from the existing context `existing-context`:

```bash
$ docker context create my-context --from existing-context
```

If the `--from` option is not set, the `context` is created from the current context:

```bash
$ docker context create my-context
```

This can be used to create a context out of an existing `DOCKER_HOST` based script:

```bash
$ source my-setup-script.sh
$ docker context create my-context
```

To source only the `docker` endpoint configuration from an existing context
use the `--docker from=<context-name>` option. The example below creates a
new context named `my-context` using the docker endpoint configuration from
the existing context `existing-context` and a kubernetes configuration sourced
from the file `/home/me/my-kube-config`:

```bash
$ docker context create my-context \
      --docker from=existing-context \
      --kubernetes config-file=/home/me/my-kube-config
```

To source only the `kubernetes` configuration from an existing context use the
`--kubernetes from=<context-name>` option. The example below creates a new
context named `my-context` using the kuberentes configuration from the existing
context `existing-context` and a docker endpoint of `/var/run/docker.sock`:

```bash
$ docker context create my-context \
      --docker host=/var/run/docker.sock \
      --kubernetes from=existing-context
```

Docker and Kubernetes endpoints configurations, as well as default stack
orchestrator and description can be modified with `docker context update`
