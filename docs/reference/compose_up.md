
## Description

Builds, (re)creates, starts, and attaches to containers for a service.

Unless they are already running, this command also starts any linked services.

The `docker compose up` command aggregates the output of each container (liked `docker compose logs --follow` does). 
When the command exits, all containers are stopped. Running `docker compose up --detach` starts the containers in the 
background and leaves them running.

If there are existing containers for a service, and the service’s configuration or image was changed after the 
container’s creation, `docker compose up` picks up the changes by stopping and recreating the containers 
(preserving mounted volumes). To prevent Compose from picking up changes, use the `--no-recreate` flag.

If you want to force Compose to stop and recreate all containers, use the `--force-recreate` flag.

If the process encounters an error, the exit code for this command is `1`.
If the process is interrupted using `SIGINT` (ctrl + C) or `SIGTERM`, the containers are stopped, and the exit code is `0`.
