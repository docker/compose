<!--[metadata]>
+++
title = "stop"
description = "Stops running containers without removing them. "
keywords = ["fig, composition, compose, docker, orchestration, cli, stop"]
[menu.main]
identifier="stop.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# stop

```
Usage: stop [options] [SERVICE...]

Options:
-t, --timeout TIMEOUT      Specify a shutdown timeout in seconds (default: 10).
```

Stops running containers without removing them. They can be started again with
`docker-compose start`.
