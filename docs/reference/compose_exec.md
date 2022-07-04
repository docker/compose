# docker compose exec

<!---MARKER_GEN_START-->
Execute a command in a running container.

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `-d`, `--detach` |  |  | Detached mode: Run command in the background. |
| `-e`, `--env` | `stringArray` |  | Set environment variables |
| `--index` | `int` | `1` | index of the container if there are multiple instances of a service [default: 1]. |
| `-T`, `--no-TTY` |  |  | Disable pseudo-TTY allocation. By default `docker compose exec` allocates a TTY. |
| `--privileged` |  |  | Give extended privileges to the process. |
| `-u`, `--user` | `string` |  | Run the command as this user. |
| `-w`, `--workdir` | `string` |  | Path to workdir directory for this command. |


<!---MARKER_GEN_END-->

## Description

This is the equivalent of `docker exec` targeting a Compose service.

With this subcommand you can run arbitrary commands in your services. Commands are by default allocating a TTY, so
you can use a command such as `docker compose exec web sh` to get an interactive prompt.
