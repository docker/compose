# docker compose exec

<!---MARKER_GEN_START-->
This is the equivalent of `docker exec` targeting a Compose service.

With this subcommand, you can run arbitrary commands in your services. Commands allocate a TTY by default, so
you can use a command such as `docker compose exec web sh` to get an interactive prompt.

By default, Compose will enter container in interactive mode and allocate a TTY, while the equivalent `docker exec`
command requires passing `--interactive --tty` flags to get the same behavior. Compose also support those two flags
to offer a smooth migration between commands, whenever they are no-op by default. Still, `interactive` can be used to
force disabling interactive mode (`--interactive=false`), typically when `docker compose exec` command is used inside
a script.

### Options

| Name              | Type          | Default | Description                                                                      |
|:------------------|:--------------|:--------|:---------------------------------------------------------------------------------|
| `-d`, `--detach`  | `bool`        |         | Detached mode: Run command in the background                                     |
| `--dry-run`       | `bool`        |         | Execute command in dry run mode                                                  |
| `-e`, `--env`     | `stringArray` |         | Set environment variables                                                        |
| `--index`         | `int`         | `0`     | Index of the container if service has multiple replicas                          |
| `-T`, `--no-tty`  | `bool`        | `true`  | Disable pseudo-TTY allocation. By default `docker compose exec` allocates a TTY. |
| `--privileged`    | `bool`        |         | Give extended privileges to the process                                          |
| `-u`, `--user`    | `string`      |         | Run the command as this user                                                     |
| `-w`, `--workdir` | `string`      |         | Path to workdir directory for this command                                       |


<!---MARKER_GEN_END-->

## Description

This is the equivalent of `docker exec` targeting a Compose service.

With this subcommand, you can run arbitrary commands in your services. Commands allocate a TTY by default, so
you can use a command such as `docker compose exec web sh` to get an interactive prompt.

By default, Compose will enter container in interactive mode and allocate a TTY, while the equivalent `docker exec` 
command requires passing `--interactive --tty` flags to get the same behavior. Compose also support those two flags
to offer a smooth migration between commands, whenever they are no-op by default. Still, `interactive` can be used to 
force disabling interactive mode (`--interactive=false`), typically when `docker compose exec` command is used inside 
a script.