<!--[metadata]>
+++
title = "rm"
description = "Removes stopped service containers."
keywords = ["fig, composition, compose, docker, orchestration, cli,  rm"]
[menu.main]
identifier="rm.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# rm

```
Usage: rm [options] [SERVICE...]

Options:
-f, --force   Don't ask to confirm removal
-v            Remove volumes associated with containers
-a, --all     Also remove one-off containers
```

Removes stopped service containers.

By default, volumes attached to containers will not be removed. You can see all
volumes with `docker volume ls`.

Any data which is not in a volume will be lost.
