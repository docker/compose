Docker Compose
==============
*(Previously known as Fig)*

Compose is a tool for defining and running multi-container applications with
Docker. With Compose, you define a multi-container application in a single
file, then spin your application up in a single command which does everything
that needs to be done to get it running.

Compose is great for development environments, staging servers, and CI. We don't
recommend that you use it in production yet.

Using Compose is basically a three-step process.

1. Define your app's environment with a `Dockerfile` so it can be
reproduced anywhere.
2. Define the services that make up your app in `docker-compose.yml` so
they can be run together in an isolated environment:
3. Lastly, run `docker-compose up` and Compose will start and run your entire app.

A `docker-compose.yml` looks like this:

    web:
      build: .
      ports:
       - "5000:5000"
      volumes:
       - .:/code
      links:
       - redis
    redis:
      image: redis

Compose has commands for managing the whole lifecycle of your application:

 * Start, stop and rebuild services
 * View the status of running services
 * Stream the log output of running services
 * Run a one-off command on a service

Installation and documentation
------------------------------

- Full documentation is available on [Docker's website](http://docs.docker.com/compose/).
- If you have any questions, you can talk in real-time with other developers in the #docker-compose IRC channel on Freenode. [Click here to join using IRCCloud.](https://www.irccloud.com/invite?hostname=irc.freenode.net&channel=%23docker-compose)

Contributing
------------

[![Build Status](http://jenkins.dockerproject.org/buildStatus/icon?job=Compose%20Master)](http://jenkins.dockerproject.org/job/Compose%20Master/)

Want to help build Compose? Check out our [contributing documentation](https://github.com/docker/compose/blob/master/CONTRIBUTING.md).

Releasing
---------

Releases are built by maintainers, following an outline of the [release process](https://github.com/docker/compose/blob/master/RELEASE_PROCESS.md).
