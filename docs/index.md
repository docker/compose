---
layout: default
title: Compose | Fast, isolated development environments using Docker
---

<strong class="strapline">Fast, isolated development environments using Docker.</strong>

Define your app's environment with a `Dockerfile` so it can be reproduced anywhere:

    FROM python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

Define the services that make up your app in `docker-compose.yml` so they can be run together in an isolated environment:

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

Then type `docker-compose up`, and Compose will start and run your entire app:

![example docker-compose run](https://orchardup.com/static/images/docker-compose-example-large.gif)

There are commands to:

 - start, stop and rebuild services
 - view the status of running services
 - tail running services' log output
 - run a one-off command on a service


Quick start
-----------

Let's get a basic Python web app running on Compose. It assumes a little knowledge of Python, but the concepts should be clear if you're not familiar with it.

First, [install Docker and Compose](install.html).

You'll want to make a directory for the project:

    $ mkdir docker-composetest
    $ cd docker-composetest

Inside this directory, create `app.py`, a simple web app that uses the Flask framework and increments a value in Redis:

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

We define our Python dependencies in a file called `requirements.txt`:

    flask
    redis

Next, we want to create a Docker image containing all of our app's dependencies. We specify how to build one using a file called `Dockerfile`:

    FROM python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

This tells Docker to install Python, our code and our Python dependencies inside a Docker image. For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

We then define a set of services using `docker-compose.yml`:

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

 - `web`, which is built from `Dockerfile` in the current directory. It also says to run the command `python app.py` inside the image, forward the exposed port 5000 on the container to port 5000 on the host machine, connect up the Redis service, and mount the current directory inside the container so we can work on code without having to rebuild the image.
 - `redis`, which uses the public image [redis](https://registry.hub.docker.com/_/redis/). 

Now if we run `docker-compose up`, it'll pull a Redis image, build an image for our own code, and start everything up:

    $ docker-compose up
    Pulling image redis...
    Building web...
    Starting docker-composetest_redis_1...
    Starting docker-composetest_web_1...
    redis_1 | [8] 02 Jan 18:43:35.576 # Server started, Redis version 2.8.3
    web_1   |  * Running on http://0.0.0.0:5000/

The web app should now be listening on port 5000 on your docker daemon (if you're using boot2docker, `boot2docker ip` will tell you its address).

If you want to run your services in the background, you can pass the `-d` flag to `docker-compose up` and use `docker-compose ps` to see what is currently running:

    $ docker-compose up -d
    Starting docker-composetest_redis_1...
    Starting docker-composetest_web_1...
    $ docker-compose ps
            Name                 Command            State       Ports
    -------------------------------------------------------------------
    docker-composetest_redis_1   /usr/local/bin/run         Up
    docker-composetest_web_1     /bin/sh -c python app.py   Up      5000->5000/tcp

`docker-compose run` allows you to run one-off commands for your services. For example, to see what environment variables are available to the `web` service:

    $ docker-compose run web env


See `docker-compose --help` other commands that are available.

If you started Compose with `docker-compose up -d`, you'll probably want to stop your services once you've finished with them:

    $ docker-compose stop

That's more-or-less how Compose works. See the reference section below for full details on the commands, condocker-composeuration file and environment variables. If you have any thoughts or suggestions, [open an issue on GitHub](https://github.com/docker/docker-compose).
