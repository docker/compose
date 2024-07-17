# docker compose run

<!---MARKER_GEN_START-->
Runs a one-time command against a service.

The following command starts the `web` service and runs `bash` as its command:

```console
$ docker compose run web bash
```

Commands you use with run start in new containers with configuration defined by that of the service,
including volumes, links, and other details. However, there are two important differences:

First, the command passed by `run` overrides the command defined in the service configuration. For example, if the
`web` service configuration is started with `bash`, then `docker compose run web python app.py` overrides it with
`python app.py`.

The second difference is that the `docker compose run` command does not create any of the ports specified in the
service configuration. This prevents port collisions with already-open ports. If you do want the service’s ports
to be created and mapped to the host, specify the `--service-ports`

```console
$ docker compose run --service-ports web python manage.py shell
```

Alternatively, manual port mapping can be specified with the `--publish` or `-p` options, just as when using docker run:

```console
$ docker compose run --publish 8080:80 -p 2022:22 -p 127.0.0.1:2021:21 web python manage.py shell
```

If you start a service configured with links, the run command first checks to see if the linked service is running
and starts the service if it is stopped. Once all the linked services are running, the run executes the command you
passed it. For example, you could run:

```console
$ docker compose run db psql -h db -U docker
```

This opens an interactive PostgreSQL shell for the linked `db` container.

If you do not want the run command to start linked containers, use the `--no-deps` flag:

```console
$ docker compose run --no-deps web python manage.py shell
```

If you want to remove the container after running while overriding the container’s restart policy, use the `--rm` flag:

```console
$ docker compose run --rm web python manage.py db upgrade
```

This runs a database upgrade script, and removes the container when finished running, even if a restart policy is
specified in the service configuration.

### Options

| Name                    | Type          | Default | Description                                                                      |
|:------------------------|:--------------|:--------|:---------------------------------------------------------------------------------|
| `--build`               | `bool`        |         | Build image before starting container                                            |
| `--cap-add`             | `list`        |         | Add Linux capabilities                                                           |
| `--cap-drop`            | `list`        |         | Drop Linux capabilities                                                          |
| `-d`, `--detach`        | `bool`        |         | Run container in background and print container ID                               |
| `--dry-run`             | `bool`        |         | Execute command in dry run mode                                                  |
| `--entrypoint`          | `string`      |         | Override the entrypoint of the image                                             |
| `-e`, `--env`           | `stringArray` |         | Set environment variables                                                        |
| `-i`, `--interactive`   | `bool`        | `true`  | Keep STDIN open even if not attached                                             |
| `-l`, `--label`         | `stringArray` |         | Add or override a label                                                          |
| `--name`                | `string`      |         | Assign a name to the container                                                   |
| `-T`, `--no-TTY`        | `bool`        | `true`  | Disable pseudo-TTY allocation (default: auto-detected)                           |
| `--no-deps`             | `bool`        |         | Don't start linked services                                                      |
| `-p`, `--publish`       | `stringArray` |         | Publish a container's port(s) to the host                                        |
| `--quiet-pull`          | `bool`        |         | Pull without printing progress information                                       |
| `--remove-orphans`      | `bool`        |         | Remove containers for services not defined in the Compose file                   |
| `--rm`                  | `bool`        |         | Automatically remove the container when it exits                                 |
| `-P`, `--service-ports` | `bool`        |         | Run command with all service's ports enabled and mapped to the host              |
| `--use-aliases`         | `bool`        |         | Use the service's network useAliases in the network(s) the container connects to |
| `-u`, `--user`          | `string`      |         | Run as specified username or uid                                                 |
| `-v`, `--volume`        | `stringArray` |         | Bind mount a volume                                                              |
| `-w`, `--workdir`       | `string`      |         | Working directory inside the container                                           |


<!---MARKER_GEN_END-->

## Description

Runs a one-time command against a service.

The following command starts the `web` service and runs `bash` as its command:

```console
$ docker compose run web bash
```

Commands you use with run start in new containers with configuration defined by that of the service,
including volumes, links, and other details. However, there are two important differences:

First, the command passed by `run` overrides the command defined in the service configuration. For example, if the
`web` service configuration is started with `bash`, then `docker compose run web python app.py` overrides it with
`python app.py`.

The second difference is that the `docker compose run` command does not create any of the ports specified in the
service configuration. This prevents port collisions with already-open ports. If you do want the service’s ports
to be created and mapped to the host, specify the `--service-ports`

```console
$ docker compose run --service-ports web python manage.py shell
```

Alternatively, manual port mapping can be specified with the `--publish` or `-p` options, just as when using docker run:

```console
$ docker compose run --publish 8080:80 -p 2022:22 -p 127.0.0.1:2021:21 web python manage.py shell
```

If you start a service configured with links, the run command first checks to see if the linked service is running
and starts the service if it is stopped. Once all the linked services are running, the run executes the command you
passed it. For example, you could run:

```console
$ docker compose run db psql -h db -U docker
```

This opens an interactive PostgreSQL shell for the linked `db` container.

If you do not want the run command to start linked containers, use the `--no-deps` flag:

```console
$ docker compose run --no-deps web python manage.py shell
```

If you want to remove the container after running while overriding the container’s restart policy, use the `--rm` flag:

```console
$ docker compose run --rm web python manage.py db upgrade
```

This runs a database upgrade script, and removes the container when finished running, even if a restart policy is
specified in the service configuration.
