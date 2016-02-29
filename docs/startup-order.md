<!--[metadata]>
+++
title = "Controlling startup order"
description = "How to control service startup order in Docker Compose"
keywords = "documentation, docs,  docker, compose, startup, order"
[menu.main]
parent="workw_compose"
weight=90
+++
<![end-metadata]-->

# Controlling startup order in Compose

You can control the order of service startup with the
[depends_on](compose-file.md#depends-on) option. Compose always starts
containers in dependency order, where dependencies are determined by
`depends_on`, `links`, `volumes_from` and `network_mode: "service:..."`.

However, Compose will not wait until a container is "ready" (whatever that means
for your particular application) - only until it's running. There's a good
reason for this.

The problem of waiting for a database (for example) to be ready is really just
a subset of a much larger problem of distributed systems. In production, your
database could become unavailable or move hosts at any time. Your application
needs to be resilient to these types of failures.

To handle this, your application should attempt to re-establish a connection to
the database after a failure. If the application retries the connection,
it should eventually be able to connect to the database.

The best solution is to perform this check in your application code, both at
startup and whenever a connection is lost for any reason. However, if you don't
need this level of resilience, you can work around the problem with a wrapper
script:

-   Use a tool such as [wait-for-it](https://github.com/vishnubob/wait-for-it)
    or [dockerize](https://github.com/jwilder/dockerize). These are small
    wrapper scripts which you can include in your application's image and will
    poll a given host and port until it's accepting TCP connections.

    Supposing your application's image has a `CMD` set in its Dockerfile, you
    can wrap it by setting the entrypoint in `docker-compose.yml`:

        version: "2"
        services:
          web:
            build: .
            ports:
              - "80:8000"
            depends_on:
              - "db"
            entrypoint: ./wait-for-it.sh db:5432
          db:
            image: postgres

-   Write your own wrapper script to perform a more application-specific health
    check. For example, you might want to wait until Postgres is definitely
    ready to accept commands:

        #!/bin/bash

        set -e

        host="$1"
        shift
        cmd="$@"

        until psql -h "$host" -U "postgres" -c '\l'; do
          >&2 echo "Postgres is unavailable - sleeping"
          sleep 1
        done

        >&2 echo "Postgres is up - executing command"
        exec $cmd

    You can use this as a wrapper script as in the previous example, by setting
    `entrypoint: ./wait-for-postgres.sh db`.


## Compose documentation

- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with WordPress](wordpress.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
