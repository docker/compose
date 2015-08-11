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


## Extending services in Compose

Docker Compose's `extends` keyword enables sharing of common configurations
among different files, or even different projects entirely. Extending services
is useful if you have several applications that reuse commonly-defined services.
Using `extends` you can define a service in one place and refer to it from
anywhere.

Alternatively, you can deploy the same application to multiple environments with
a slightly different set of services in each case (or with changes to the
configuration of some services). Moreover, you can do so without copy-pasting
the configuration around.

### Understand the extends configuration

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

For full details on how to use `extends`, refer to the [reference](#reference).

### Example use case

In this example, you’ll repurpose the example app from the [quick start
guide](/). (If you're not familiar with Compose, it's recommended that
you go through the quick start first.) This example assumes you want to use
Compose both to develop an application locally and then deploy it to a
production environment.

The local and production environments are similar, but there are some
differences. In development, you mount the application code as a volume so that
it can pick up changes; in production, the code should be immutable from the
outside. This ensures it’s not accidentally changed. The development environment
uses a local Redis container, but in production another team manages the Redis
service, which is listening at `redis-production.example.com`.

To configure with `extends` for this sample, you must:

1.  Define the web application as a Docker image in `Dockerfile` and a Compose
    service in `common.yml`.

2.  Define the development environment in the standard Compose file,
    `docker-compose.yml`.

    - Use `extends` to pull in the web service.
    - Configure a volume to enable code reloading.
    - Create an additional Redis service for the application to use locally.

3.  Define the production environment in a third Compose file, `production.yml`.

    - Use `extends` to pull in the web service.
    - Configure the web service to talk to the external, production Redis service.

#### Define the web app

Defining the web application requires the following:

1.  Create an `app.py` file.

    This file contains a simple Python application that uses Flask to serve HTTP
    and increments a counter in Redis:

        from flask import Flask
        from redis import Redis
        import os

        app = Flask(__name__)
        redis = Redis(host=os.environ['REDIS_HOST'], port=6379)

        @app.route('/')
        def hello():
           redis.incr('hits')
           return 'Hello World! I have been seen %s times.\n' % redis.get('hits')

        if __name__ == "__main__":
           app.run(host="0.0.0.0", debug=True)

    This code uses a `REDIS_HOST` environment variable to determine where to
    find Redis.

2.  Define the Python dependencies in a `requirements.txt` file:

        flask
        redis

3.  Create a `Dockerfile` to build an image containing the app:

        FROM python:2.7
        ADD . /code
        WORKDIR /code
        RUN pip install -r requirements.txt
        CMD python app.py

4.  Create a Compose configuration file called `common.yml`:

    This configuration defines how to run the app.

        web:
          build: .
          ports:
            - "5000:5000"

    Typically, you would have dropped this configuration into
    `docker-compose.yml` file, but in order to pull it into multiple files with
    `extends`, it needs to be in a separate file.

#### Define the development environment

1.  Create a `docker-compose.yml` file.

    The `extends` option pulls in the `web` service from the `common.yml` file
    you created in the previous section.

        web:
          extends:
            file: common.yml
            service: web
          volumes:
            - .:/code
          links:
            - redis
          environment:
            - REDIS_HOST=redis
        redis:
          image: redis

    The new addition defines a `web` service that:

    - Fetches the base configuration for `web` out of `common.yml`.
    - Adds `volumes` and `links` configuration to the base (`common.yml`)
    configuration.
    - Sets the `REDIS_HOST` environment variable to point to the linked redis
    container. This environment uses a stock `redis` image from the Docker Hub.

2.  Run `docker-compose up`.

    Compose creates, links, and starts a web and redis container linked together.
    It mounts your application code inside the web container.

3.  Verify that the code is mounted by changing the message in
    `app.py`&mdash;say, from `Hello world!` to `Hello from Compose!`.

    Don't forget to refresh your browser to see the change!

#### Define the production environment

You are almost done. Now, define your production environment:

1.  Create a `production.yml` file.

    As with `docker-compose.yml`, the `extends` option pulls in the `web` service
    from `common.yml`.

        web:
          extends:
            file: common.yml
            service: web
          environment:
            - REDIS_HOST=redis-production.example.com

2.  Run `docker-compose -f production.yml up`.

    Compose creates *just* a web container and configures the Redis connection via
    the `REDIS_HOST` environment variable. This variable points to the production
    Redis instance.

    > **Note**: If you try to load up the webapp in your browser you'll get an
    > error&mdash;`redis-production.example.com` isn't actually a Redis server.

You've now done a basic `extends` configuration. As your application develops,
you can make any necessary changes to the web service in `common.yml`. Compose
picks up both the development and production environments when you next run
`docker-compose`. You don't have to do any copy-and-paste, and you don't have to
manually keep both environments in sync.


### Reference

You can use `extends` on any service together with other configuration keys. It
always expects a dictionary that should always contain the key: `service` and optionally the `file` key.

The `file` key specifies the location of a Compose configuration file defining
the extension. The `file` value can be an absolute or relative path. If you
specify a relative path, Docker Compose treats it as relative to the location
of the current file. If you don't specify a `file`, Compose looks in the
current configuration file.

The `service` key specifies the name of the service to extend, for example `web`
or `database`.

You can extend a service that itself extends another. You can extend
indefinitely. Compose does not support circular references and `docker-compose`
returns an error if it encounters them.

#### Adding and overriding configuration

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

    # original service
    build: .

    # local service
    image: redis

    # result
    image: redis

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

## Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](/reference)
- [Yaml file reference](yml.md)
- [Compose command line completion](completion.md)
