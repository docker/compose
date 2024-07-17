# docker compose exec

<!---MARKER_GEN_START-->
This is the equivalent of `docker exec` targeting a Compose service.

With this subcommand, you can run arbitrary commands in your services. Commands allocate a TTY by default, so
you can use a command such as `docker compose exec web sh` to get an interactive prompt.

### Options

| Name              | Type          | Default | Description                                                                      |
|:------------------|:--------------|:--------|:---------------------------------------------------------------------------------|
| `-d`, `--detach`  | `bool`        |         | Detached mode: Run command in the background                                     |
| `--dry-run`       | `bool`        |         | Execute command in dry run mode                                                  |
| `-e`, `--env`     | `stringArray` |         | Set environment variables                                                        |
| `--index`         | `int`         | `0`     | Index of the container if service has multiple replicas                          |
| `-T`, `--no-TTY`  | `bool`        | `true`  | Disable pseudo-TTY allocation. By default `docker compose exec` allocates a TTY. |
| `--privileged`    | `bool`        |         | Give extended privileges to the process                                          |
| `-u`, `--user`    | `string`      |         | Run the command as this user                                                     |
| `-w`, `--workdir` | `string`      |         | Path to workdir directory for this command                                       |


<!---MARKER_GEN_END-->

## Description

This is the equivalent of `docker exec` targeting a Compose service.

With this subcommand, you can run arbitrary commands in your services. Commands allocate a TTY by default, so
you can use a command such as `docker compose exec web sh` to get an interactive prompt.
