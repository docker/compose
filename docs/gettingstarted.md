<!--[metadata]>
+++
title = "Getting Started"
description = "Getting started with Docker Compose"
keywords = ["documentation, docs,  docker, compose, orchestration, containers"]
[menu.main]
parent="workw_compose"
weight=-85
+++
<![end-metadata]-->


# Getting Started

On this page you build a simple Python web application running on Docker Compose. The
application uses the Flask framework and increments a value in Redis. While the
sample uses Python, the concepts demonstrated here should be understandable even
if you're not familiar with it.

## Prerequisites

Make sure you have already
[installed both Docker Engine and Docker Compose](install.md). You
don't need to install Python, it is provided by a Docker image.

## Step 1: Setup

1. Create a directory for the project:

        $ mkdir composetest
        $ cd composetest

2. With your favorite text editor create a file called `app.py` in your project
   directory.

        from flask import Flask
        from redis import Redis

        app = Flask(__name__)
        redis = Redis(host='redis', port=6379)

        @app.route('/')
        def hello():
            redis.incr('hits')
            return 'Hello World! I have been seen %s times.' % redis.get('hits')

        if __name__ == "__main__":
            app.run(host="0.0.0.0", debug=True)

3. Create another file called `requirements.txt` in your project directory and
   add the following:

        flask
        redis

   These define the applications dependencies.

## Step 2: Create a Docker image

In this step, you build a new Docker image. The image contains all the
dependencies the Python application requires, including Python itself.

1. In your project directory create a file named `Dockerfile` and add the
   following:

        FROM python:2.7
        ADD . /code
        WORKDIR /code
        RUN pip install -r requirements.txt
        CMD python app.py

  This tells Docker to:

  * Build an image starting with the Python 2.7 image.
  * Add the current directory `.` into the path `/code` in the image.
  * Set the working directory to `/code`.
  * Install the Python dependencies.
  * Set the default command for the container to `python app.py`

  For more information on how to write Dockerfiles, see the [Docker user guide](/engine/userguide/containers/dockerimages.md#building-an-image-from-a-dockerfile) and the [Dockerfile reference](/engine/reference/builder.md).

2. Build the image.

        $ docker build -t web .

  This command builds an image named `web` from the contents of the current
  directory. The command automatically locates the `Dockerfile`, `app.py`, and
  `requirements.txt` files.


## Step 3: Define services

Define a set of services using `docker-compose.yml`:

1. Create a file called docker-compose.yml in your project directory and add
   the following:


        version: '2'
        services:
          web:
            build: .
            ports:
             - "5000:5000"
            volumes:
             - .:/code
            depends_on:
             - redis
          redis:
            image: redis

This Compose file defines two services, `web` and `redis`. The web service:

* Builds from the `Dockerfile` in the current directory.
* Forwards the exposed port 5000 on the container to port 5000 on the host machine.
* Mounts the project directory on the host to `/code` inside the container allowing you to modify the code without having to rebuild the image.
* Links the web service to the Redis service.

The `redis` service uses the latest public [Redis](https://registry.hub.docker.com/_/redis/) image pulled from the Docker Hub registry.

## Step 4: Build and run your app with Compose

1. From your project directory, start up your application.

        $ docker-compose up
        Pulling image redis...
        Building web...
        Starting composetest_redis_1...
        Starting composetest_web_1...
        redis_1 | [8] 02 Jan 18:43:35.576 # Server started, Redis version 2.8.3
        web_1   |  * Running on http://0.0.0.0:5000/
        web_1   |  * Restarting with stat

   Compose pulls a Redis image, builds an image for your code, and start the
   services you defined.

2. Enter `http://0.0.0.0:5000/` in a browser to see the application running.

   If you're using Docker on Linux natively, then the web app should now be
   listening on port 5000 on your Docker daemon host. If `http://0.0.0.0:5000`
   doesn't resolve, you can also try `http://localhost:5000`.

   If you're using Docker Machine on a Mac, use `docker-machine ip MACHINE_VM` to get
   the IP address of your Docker host. Then, `open http://MACHINE_VM_IP:5000` in a
   browser.

   You should see a message in your browser saying:

   `Hello World! I have been seen 1 times.`

3. Refresh the page.

   The number should increment.

## Step 5: Experiment with some other commands

If you want to run your services in the background, you can pass the `-d` flag
(for "detached" mode) to `docker-compose up` and use `docker-compose ps` to
see what is currently running:

        $ docker-compose up -d
        Starting composetest_redis_1...
        Starting composetest_web_1...
        $ docker-compose ps
        Name                 Command            State       Ports
        -------------------------------------------------------------------
        composetest_redis_1   /usr/local/bin/run         Up
        composetest_web_1     /bin/sh -c python app.py   Up      5000->5000/tcp

The `docker-compose run` command allows you to run one-off commands for your
services. For example, to see what environment variables are available to the
`web` service:

        $ docker-compose run web env

See `docker-compose --help` to see other available commands. You can also install [command completion](completion.md) for the bash and zsh shell, which will also show you available commands.

If you started Compose with `docker-compose up -d`, you'll probably want to stop
your services once you've finished with them:

        $ docker-compose stop

At this point, you have seen the basics of how Compose works.


## Where to go next

- Next, try the quick start guide for [Django](django.md),
  [Rails](rails.md), or [WordPress](wordpress.md).
- [Explore the full list of Compose commands](./reference/index.md)
- [Compose configuration file reference](compose-file.md)
