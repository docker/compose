<!--[metadata]>
+++
title = "run"
description = "Runs a one-off command on a service."
keywords = ["fig, composition, compose, docker, orchestration, cli,  run"]
[menu.main]
identifier="run.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# run

```
Usage: run [options] [-e KEY=VAL...] SERVICE [COMMAND] [ARGS...]

Options:
-d                    Detached mode: Run container in the background, print
                          new container name.
--entrypoint CMD      Override the entrypoint of the image.
-e KEY=VAL            Set an environment variable (can be used multiple times)
-u, --user=""         Run as specified username or uid
--no-deps             Don't start linked services.
--rm                  Remove container after run. Ignored in detached mode.
-p, --publish=[]      Publish a container's port(s) to the host
--service-ports       Run command with the service's ports enabled and mapped to the host.
-T                    Disable pseudo-tty allocation. By default `docker-compose run` allocates a TTY.
```

Runs a one-time command against a service. For example, the following command starts the `web` service and runs `bash` as its command.

    $ docker-compose run web bash

Commands you use with `run` start in new containers with the same configuration as defined by the service' configuration. This means the container has the same volumes, links, as defined in the configuration file. There two differences though.

First, the command passed by `run` overrides the command defined in the service configuration. For example, if the  `web` service configuration is started with `bash`, then `docker-compose run web python app.py` overrides it with `python app.py`.

The second difference is the `docker-compose run` command does not create any of the ports specified in the service configuration. This prevents the port collisions with already open ports. If you *do want* the service's ports created and mapped to the host, specify the `--service-ports` flag:

    $ docker-compose run --service-ports web python manage.py shell

Alternatively manual port mapping can be specified. Same as when running Docker's `run` command - using `--publish` or `-p` options:

    $ docker-compose run --publish 8080:80 -p 2022:22 -p 127.0.0.1:2021:21 web python manage.py shell

If you start a service configured with links, the `run` command first checks to see if the linked service is running and starts the service if it is stopped.  Once all the linked services are running, the `run` executes the command you passed it.  So, for example, you could run:

    $ docker-compose run db psql -h db -U docker

This would open up an interactive PostgreSQL shell for the linked `db` container.

If you do not want the `run` command to start linked containers, specify the `--no-deps` flag:

    $ docker-compose run --no-deps web python manage.py shell
