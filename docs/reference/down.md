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
Usage: down [options]

Options:
    --rmi type          Remove images. Type must be one of:
                        'all': Remove all images used by any service.
                        'local': Remove only images that don't have a custom tag
                        set by the `image` field.
    -v, --volumes       Remove named volumes declared in the `volumes` section
                        of the Compose file and anonymous volumes
                        attached to containers.
    --remove-orphans    Remove containers for services not defined in the
                        Compose file
```

Stops containers and removes containers, networks, volumes, and images
created by `up`.

By default, the only things removed are:

- Containers for services defined in the Compose file
- Networks defined in the `networks` section of the Compose file
- The default network, if one is used

Networks and volumes defined as `external` are never removed.
