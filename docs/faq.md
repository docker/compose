<!--[metadata]>
+++
title = "Frequently Asked Questions"
description = "Docker Compose FAQ"
keywords = "documentation, docs,  docker, compose, faq"
[menu.main]
identifier="faq.compose"
parent="workw_compose"
weight=90
+++
<![end-metadata]-->

# Frequently asked questions

If you don’t see your question here, feel free to drop by `#docker-compose` on
freenode IRC and ask the community.


## Can I control service startup order?

Yes - see [Controlling startup order](startup-order.md).


## Why do my services take 10 seconds to recreate or stop?

Compose stop attempts to stop a container by sending a `SIGTERM`. It then waits
for a [default timeout of 10 seconds](./reference/stop.md).  After the timeout,
a `SIGKILL` is sent to the container to forcefully kill it.  If you
are waiting for this timeout, it means that your containers aren't shutting down
when they receive the `SIGTERM` signal.

There has already been a lot written about this problem of
[processes handling signals](https://medium.com/@gchudnov/trapping-signals-in-docker-containers-7a57fdda7d86)
in containers.

To fix this problem, try the following:

* Make sure you're using the JSON form of `CMD` and `ENTRYPOINT`
in your Dockerfile.

  For example use `["program", "arg1", "arg2"]` not `"program arg1 arg2"`.
  Using the string form causes Docker to run your process using `bash` which
  doesn't handle signals properly. Compose always uses the JSON form, so don't
  worry if you override the command or entrypoint in your Compose file.

* If you are able, modify the application that you're running to
add an explicit signal handler for `SIGTERM`.

* Set the `stop_signal` to a signal which the application knows how to handle:

      web:
        build: .
        stop_signal: SIGINT

* If you can't modify the application, wrap the application in a lightweight init
system (like [s6](http://skarnet.org/software/s6/)) or a signal proxy (like
[dumb-init](https://github.com/Yelp/dumb-init) or
[tini](https://github.com/krallin/tini)).  Either of these wrappers take care of
handling `SIGTERM` properly.

## How do I run multiple copies of a Compose file on the same host?

Compose uses the project name to create unique identifiers for all of a
project's  containers and other resources. To run multiple copies of a project,
set a custom project name using the [`-p` command line
option](./reference/overview.md) or the [`COMPOSE_PROJECT_NAME`
environment variable](./reference/envvars.md#compose-project-name).

## What's the difference between `up`, `run`, and `start`?

Typically, you want `docker-compose up`. Use `up` to start or restart all the
services defined in a `docker-compose.yml`. In the default "attached"
mode, you'll see all the logs from all the containers. In "detached" mode (`-d`),
Compose exits after starting the containers, but the containers continue to run
in the background.

The `docker-compose run` command is for running "one-off" or "adhoc" tasks. It
requires the service name you want to run and only starts containers for services
that the running service depends on. Use `run` to run tests or perform
an administrative task such as removing or adding data to a data volume
container. The `run` command acts like `docker run -ti` in that it opens an
interactive terminal to the container and returns an exit status matching the
exit status of the process in the container.

The `docker-compose start` command is useful only to restart containers
that were previously created, but were stopped. It never creates new
containers.

## Can I use json instead of yaml for my Compose file?

Yes. [Yaml is a superset of json](http://stackoverflow.com/a/1729545/444646) so
any JSON file should be valid Yaml.  To use a JSON file with Compose,
specify the filename to use, for example:

```bash
docker-compose -f docker-compose.json up
```

## Should I include my code with `COPY`/`ADD` or a volume?

You can add your code to the image using `COPY` or `ADD` directive in a
`Dockerfile`.  This is useful if you need to relocate your code along with the
Docker image, for example when you're sending code to another environment
(production, CI, etc).

You should use a `volume` if you want to make changes to your code and see them
reflected immediately, for example when you're developing code and your server
supports hot code reloading or live-reload.

There may be cases where you'll want to use both. You can have the image
include the code using a `COPY`, and use a `volume` in your Compose file to
include the code from the host during development. The volume overrides
the directory contents of the image.

## Where can I find example compose files?

There are [many examples of Compose files on
github](https://github.com/search?q=in%3Apath+docker-compose.yml+extension%3Ayml&type=Code).


## Compose documentation

- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with WordPress](wordpress.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
