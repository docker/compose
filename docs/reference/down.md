<!--[metadata]>
+++
title = "down"
description = "down"
keywords = ["fig, composition, compose, docker, orchestration, cli,  down"]
[menu.main]
identifier="down.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# down

```
Stop containers and remove containers, networks, volumes, and images
created by `up`. Only containers and networks are removed by default.

Usage: down [options]

Options:
    --rmi type          Remove images, type may be one of: 'all' to remove
                        all images, or 'local' to remove only images that
                        don't have an custom name set by the `image` field
    -v, --volumes       Remove data volumes

    --remove-orphans    Remove containers for services not defined in the
                        Compose file
```
