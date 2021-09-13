
## Description

Lists containers for a Compose project, with current status and exposed ports.

```console
$ docker compose ps
NAME                SERVICE             STATUS              PORTS
example_foo_1       foo                 running (healthy)   0.0.0.0:8000->80/tcp
example_bar_1       bar                 exited (1)          
```
