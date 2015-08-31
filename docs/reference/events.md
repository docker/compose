<!--[metadata]>
+++
title = "events"
description = "Receive real time events from containers."
keywords = ["fig, composition, compose, docker, orchestration, cli, events"]
[menu.main]
identifier="events.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# events

```
Usage: events [options] [SERVICE...]

Options:
    --json      Output events as a stream of json objects
```

Stream container events for every container in the project.

With the `--json` flag, a json object will be printed one per line with the
format:

```
{
    "service": "web",
    "event": "create",
    "container": "213cf75fc39a",
    "image": "alpine:edge",
    "time": "2015-11-20T18:01:03.615550",
}
```
