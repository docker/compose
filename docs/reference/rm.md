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
```

Removes stopped service containers. _This removes the containers completely, you will lose any data that was contained on them_.
