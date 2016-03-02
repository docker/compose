<!--[metadata]>
+++
title = "create"
description = "Create creates containers for a service."
keywords = ["fig, composition, compose, docker, orchestration, cli, create"]
[menu.main]
identifier="create.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# create

```
Creates containers for a service.

Usage: create [options] [SERVICE...]

Options:
    --force-recreate       Recreate containers even if their configuration and
                           image haven't changed. Incompatible with --no-recreate.
    --no-recreate          If containers already exist, don't recreate them.
                           Incompatible with --force-recreate.
    --no-build             Don't build an image, even if it's missing.
    --build                Build images before creating containers.
```
