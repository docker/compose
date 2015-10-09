<!--[metadata]>
+++
title = "Extending services in Compose"
description = "How to use Docker Compose's extends keyword to share configuration between files and projects"
keywords = ["fig, composition, compose, docker, orchestration, documentation, docs"]
[menu.main]
parent="smn_workw_compose"
weight=2
+++
<![end-metadata]-->


## Extending services and Compose files

Compose supports two ways to sharing common configuration and
extend a service with that shared configuration.

1. Extending individual services with [the `extends` field](#extending-services)
2. Extending entire compositions by
   [exnteding compose files](#extending-compose-files)

### Extending services

Docker Compose's `extends` keyword enables sharing of common configurations
among different files, or even different projects entirely. Extending services
is useful if you have several services that reuse a common set of configuration
options. Using `extends` you can define a common set of service options in one
place and refer to it from anywhere.

> **Note:** `links` and `volumes_from` are never shared between services using
> `extends`. See
> [Adding and overriding configuration](#adding-and-overriding-configuration)
> for more information.

#### Understand the extends configuration

When defining any service in `docker-compose.yml`, you can declare that you are
extending another service like this:

    web:
      extends:
        file: common-services.yml
        service: webapp

This instructs Compose to re-use the configuration for the `webapp` service
defined in the `common-services.yml` file. Suppose that `common-services.yml`
looks like this:

    webapp:
      build: .
      ports:
        - "8000:8000"
      volumes:
        - "/data"

In this case, you'll get exactly the same result as if you wrote
`docker-compose.yml` with that `build`, `ports` and `volumes` configuration
defined directly under `web`.

You can go further and define (or re-define) configuration locally in
`docker-compose.yml`:

    web:
      extends:
        file: common-services.yml
        service: webapp
      environment:
        - DEBUG=1
      cpu_shares: 5

    important_web:
      extends: web
      cpu_shares: 10

You can also write other services and link your `web` service to them:

    web:
      extends:
        file: common-services.yml
        service: webapp
      environment:
        - DEBUG=1
      cpu_shares: 5
      links:
        - db
    db:
      image: postgres

#### Example use case

Extending an individual service is useful when you have multiple services that
have a common configuration.  In this example we have a composition that with
a web application and a queue worker. Both services use the same codebase and
share many configuration options.

In a **common.yml** we'll define the common configuration:

    app:
      build: .
      environment:
        CONFIG_FILE_PATH: /code/config
        API_KEY: xxxyyy
      cpu_shares: 5

In a **docker-compose.yml** we'll define the concrete services which use the
common configuration:


    webapp:
      extends:
        file: common.yml
        service: app
      command: /code/run_web_app
      ports:
        - 8080:8080
      links:
        - queue
        - db

    queue_worker:
      extends:
        file: common.yml
        service: app
    command: /code/run_worker
    links:
      - queue

#### Adding  and overriding configuration

Compose copies configurations from the original service over to the local one,
**except** for `links` and `volumes_from`. These exceptions exist to avoid
implicit dependencies&mdash;you always define `links` and `volumes_from`
locally. This ensures dependencies between services are clearly visible when
reading the current file. Defining these locally also ensures changes to the
referenced file don't result in breakage.

If a configuration option is defined in both the original service and the local
service, the local value either *override*s or *extend*s the definition of the
original service. This works differently for other configuration options.

For single-value options like `image`, `command` or `mem_limit`, the new value
replaces the old value. **This is the default behaviour - all exceptions are
listed below.**

    # original service
    command: python app.py

    # local service
    command: python otherapp.py

    # result
    command: python otherapp.py

In the case of `build` and `image`, using one in the local service causes
Compose to discard the other, if it was defined in the original service.

Example of image replacing build:

    # original service
    build: .

    # local service
    image: redis

    # result
    image: redis


Example of build replacing image:

    # original service
    image: redis

    # local service
    build: .

    # result
    build: .

For the **multi-value options** `ports`, `expose`, `external_links`, `dns` and
`dns_search`, Compose concatenates both sets of values:

    # original service
    expose:
      - "3000"

    # local service
    expose:
      - "4000"
      - "5000"

    # result
    expose:
      - "3000"
      - "4000"
      - "5000"

In the case of `environment` and `labels`, Compose "merges" entries together
with locally-defined values taking precedence:

    # original service
    environment:
      - FOO=original
      - BAR=original

    # local service
    environment:
      - BAR=local
      - BAZ=local

    # result
    environment:
      - FOO=original
      - BAR=local
      - BAZ=local

Finally, for `volumes` and `devices`, Compose "merges" entries together with
locally-defined bindings taking precedence:

    # original service
    volumes:
      - /original-dir/foo:/foo
      - /original-dir/bar:/bar

    # local service
    volumes:
      - /local-dir/bar:/bar
      - /local-dir/baz/:baz

    # result
    volumes:
      - /original-dir/foo:/foo
      - /local-dir/bar:/bar
      - /local-dir/baz/:baz


### Extending Compose files

> **Note:** This feature is new in `docker-compose` 1.5



## Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Getting Started](gettingstarted.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with WordPress](wordpress.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
