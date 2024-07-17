# docker compose kill

<!---MARKER_GEN_START-->
Forces running containers to stop by sending a `SIGKILL` signal. Optionally the signal can be passed, for example:

```console
$ docker-compose kill -s SIGINT
```

### Options

| Name               | Type     | Default   | Description                                                    |
|:-------------------|:---------|:----------|:---------------------------------------------------------------|
| `--dry-run`        | `bool`   |           | Execute command in dry run mode                                |
| `--remove-orphans` | `bool`   |           | Remove containers for services not defined in the Compose file |
| `-s`, `--signal`   | `string` | `SIGKILL` | SIGNAL to send to the container                                |


<!---MARKER_GEN_END-->

## Description

Forces running containers to stop by sending a `SIGKILL` signal. Optionally the signal can be passed, for example:

```console
$ docker-compose kill -s SIGINT
```
