<!--[metadata]>
+++
title = "clean"
description = "Kill and remove orphan containers."
keywords = ["fig, composition, compose, docker, orchestration, cli, clean"]
[menu.main]
identifier="clean.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# clean

```
Usage: clean [options]

Options:
--keep          Do not remove containers.
-f, --force     Don't ask to confirm removal
```

Kill and remove orphan containers.

If any container from a previous version of docker-compose.yml is running,
this container will be killed and removed.

If the container is not running, it will be removed.

Orphan containers are identified using labels.

To prevent Compose from removing containers, use the `--keep` flag.

    $ docker-compose clean --keep
