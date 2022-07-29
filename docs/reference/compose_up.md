# docker compose up

<!---MARKER_GEN_START-->
Create and start containers

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `--abort-on-container-exit` |  |  | Stops all containers if any container was stopped. Incompatible with -d |
| `--always-recreate-deps` |  |  | Recreate dependent containers. Incompatible with --no-recreate. |
| `--attach` | `stringArray` |  | Attach to service output. |
| `--attach-dependencies` |  |  | Attach to dependent containers. |
| `--build` |  |  | Build images before starting containers. |
| `-d`, `--detach` |  |  | Detached mode: Run containers in the background |
| `--exit-code-from` | `string` |  | Return the exit code of the selected service container. Implies --abort-on-container-exit |
| `--force-recreate` |  |  | Recreate containers even if their configuration and image haven't changed. |
| `--no-build` |  |  | Don't build an image, even if it's missing. |
| `--no-color` |  |  | Produce monochrome output. |
| `--no-deps` |  |  | Don't start linked services. |
| `--no-log-prefix` |  |  | Don't print prefix in logs. |
| `--no-recreate` |  |  | If containers already exist, don't recreate them. Incompatible with --force-recreate. |
| `--no-start` |  |  | Don't start the services after creating them. |
| `--pull` | `string` | `missing` | Pull image before running ("always"\|"missing"\|"never") |
| `--quiet-pull` |  |  | Pull without printing progress information. |
| `--remove-orphans` |  |  | Remove containers for services not defined in the Compose file. |
| `-V`, `--renew-anon-volumes` |  |  | Recreate anonymous volumes instead of retrieving data from the previous containers. |
| `--scale` | `stringArray` |  | Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present. |
| `-t`, `--timeout` | `int` | `10` | Use this timeout in seconds for container shutdown when attached or when containers are already running. |
| `--wait` |  |  | Wait for services to be running\|healthy. Implies detached mode. |


<!---MARKER_GEN_END-->

## Description

Builds, (re)creates, starts, and attaches to containers for a service.

Unless they are already running, this command also starts any linked services.

The `docker compose up` command aggregates the output of each container (like `docker compose logs --follow` does).
When the command exits, all containers are stopped. Running `docker compose up --detach` starts the containers in the
background and leaves them running.

If there are existing containers for a service, and the service’s configuration or image was changed after the
container’s creation, `docker compose up` picks up the changes by stopping and recreating the containers
(preserving mounted volumes). To prevent Compose from picking up changes, use the `--no-recreate` flag.

If you want to force Compose to stop and recreate all containers, use the `--force-recreate` flag.

If the process encounters an error, the exit code for this command is `1`.
If the process is interrupted using `SIGINT` (ctrl + C) or `SIGTERM`, the containers are stopped, and the exit code is `0`.
