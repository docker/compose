page_title: Compose: Multi-container orchestration for Docker
page_description: Introduction and Overview of Compose
page_keywords: documentation, docs,  docker, compose, orchestration, containers 


## Overview

Compose is a tool that allows you to orchestrate multiple Docker containers. With Compose, you can build clusters of containers which provide the resources (services, volumes, etc.) needed to build and run a complete distributed application. 

You can use Compose to build your app with containers hosted locally, or on a remote server, including cloud-based instances. Compose can also be used to deploy code to production.

Using Compose is basically a three-step process.
 
First, you define your app's environment with a `Dockerfile` so it can be reproduced anywhere:

    FROM python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

Next, you define the services that make up your app in `docker-compose.yml` so they can be run together in an isolated environment:

```yaml
web:
  build: .
  command: python app.py
  links:
   - db
  ports:
   - "8000:8000"
db:
  image: postgres
```

(No more installing Postgres on your laptop!)

Lastly, run `docker-compose up` and Compose will start and run your entire app.

Compose includes commands to:

 * Start, stop and rebuild services
 * View the status of running services
 * tail the log output of running services
 * run a one-off command on a service


## Quick start

Let's get started with a walkthrough of getting a simple Python web app running on Compose. It assumes a little knowledge of Python, but the concepts demonstrated here should be understandable even if you're not familiar with Python.

### Installation and set-up

First, [install Docker and Compose](install.html).

Next, you'll want to make a directory for the project:

    $ mkdir composetest
    $ cd composetest

Inside this directory, create `app.py`, a simple web app that uses the Flask
framework and increments a value in Redis:

```python
from flask import Flask
from redis import Redis
import os
app = Flask(__name__)
redis = Redis(host='redis', port=6379)

@app.route('/')
def hello():
    redis.incr('hits')
    return 'Hello World! I have been seen %s times.' % redis.get('hits')

if __name__ == "__main__":
    app.run(host="0.0.0.0", debug=True)
```

Next, define the Python dependencies in a file called `requirements.txt`:

    flask
    redis

### Create a Docker image

Now, create a Docker image containing all of your app's dependencies. You
specify how to build the image using a file called [`Dockerfile`](http://docs.docker.com/reference/builder/):

    FROM python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

This tells Docker to include Python, your code, and your Python dependencies in
a Docker image. For more information on how to write Dockerfiles, see the
[Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the
[Dockerfile reference](http://docs.docker.com/reference/builder/).

### Define services

Next, define a set of services using `docker-compose.yml`:

    web:
      build: .
      command: python app.py
      ports:
       - "5000:5000"
      volumes:
       - .:/code
      links:
       - redis
    redis:
      image: redis

This defines two services:

 - `web`, which is built from the `Dockerfile` in the current directory. It also
 says to run the command `python app.py` inside the image, forward the exposed
port 5000 on the container to port 5000 on the host machine, connect up the
Redis service, and mount the current directory inside the container so we can
work on code without having to rebuild the image.
 - `redis`, which uses the public image [redis](https://registry.hub.docker.com/_/redis/), which gets pulled from the
 Docker Hub registry. 

### Build and run your app with Compose

Now, when you run `docker-compose up`, Compose will pull a Redis image, build an
image for your code, and start everything up:

    $ docker-compose up
    Pulling image redis...
    Building web...
    Starting composetest_redis_1...
    Starting composetest_web_1...
    redis_1 | [8] 02 Jan 18:43:35.576 # Server started, Redis version 2.8.3
    web_1   |  * Running on http://0.0.0.0:5000/

The web app should now be listening on port 5000 on your docker daemon (if
you're using boot2docker, `boot2docker ip` will tell you its address).

If you want to run your services in the background, you can pass the `-d` flag
(for daemon mode) to `docker-compose up` and use `docker-compose ps` to see what
is currently running:

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

See `docker-compose --help` to see other available commands.

If you started Compose with `docker-compose up -d`, you'll probably want to stop
your services once you've finished with them:

    $ docker-compose stop

At this point, you have seen the basics of how Compose works. See the reference section for complete details on the commands, configuration file and environment variables.
