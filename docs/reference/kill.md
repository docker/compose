<!--[metadata]>
+++
title = "kill"
description = "Forces running containers to stop."
keywords = ["fig, composition, compose, docker, orchestration, cli,  kill"]
[menu.main]
identifier="kill.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# kill

```
Usage: kill [options] [SERVICE...]

Options:
-s SIGNAL         SIGNAL to send to the container. Default signal is SIGKILL.
```

Forces running containers to stop by sending a `SIGKILL` signal. Optionally the
signal can be passed, for example:

    $ docker-compose kill -s SIGINT
