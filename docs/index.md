page_title: Compose: Multi-container orchestration for Docker
page_description: Introduction and Overview of Compose
page_keywords: documentation, docs,  docker, compose, orchestration, containers


# Docker Compose

Compose is a tool for defining and running complex applications with Docker.
With Compose, you define a multi-container application in a single file, then
spin your application up in a single command which does everything that needs to
be done to get it running.

Compose is great for development environments, staging servers, and CI. We don't
recommend that you use it in production yet.

Using Compose is basically a three-step process.

First, you define your app's environment with a `Dockerfile` so it can be
reproduced anywhere:

```Dockerfile
FROM python:2.7
WORKDIR /code
ADD requirements.txt /code/
RUN pip install -r requirements.txt
ADD . /code
CMD python app.py
```

Next, you define the services that make up your app in `docker-compose.yml` so
they can be run together in an isolated environment:

```yaml
web:
  build: .
  links:
   - db
  ports:
   - "8000:8000"
db:
  image: postgres
```

Lastly, run `docker-compose up` and Compose will start and run your entire app.

Compose has commands for managing the whole lifecycle of your application:

 * Start, stop and rebuild services
 * View the status of running services
 * Stream the log output of running services
 * Run a one-off command on a service

## Compose documentation

- [Installing Compose](install.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)

## Quick start

Let's get started with a walkthrough of getting a simple Python web app running
on Compose. It assumes a little knowledge of Python, but the concepts
demonstrated here should be understandable even if you're not familiar with
Python.

### Installation and set-up

First, [install Docker and Compose](install.md).

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
specify how to build the image using a file called
[`Dockerfile`](http://docs.docker.com/reference/builder/):

    FROM python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

This tells Docker to include Python, your code, and your Python dependencies in
a Docker image. For more information on how to write Dockerfiles, see the
[Docker user
guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile)
and the
[Dockerfile reference](http://docs.docker.com/reference/builder/).

### Define services

Next, define a set of services using `docker-compose.yml`:

    web:
      build: .
      command: python app.py
      ports:
       - "5000:5000"
      links:
       - redis
    redis:
      image: redis

This defines two services:

 - `web`, which is built from the `Dockerfile` in the current directory. It also
   says to run the command `python app.py` inside the image, forward the exposed
   port 5000 on the container to port 5000 on the host machine, and connect up a
   Redis service.
 - `redis`, which uses the public image
   [redis](https://registry.hub.docker.com/_/redis/), which gets pulled from the
   Docker Hub registry.

### Build and run your app with Compose

Now, when you run `docker-compose up`, Compose will pull a Redis image, build an
image for your code, and start everything up:

    $ docker-compose up
    Pulling image redis...
    Building web...
    Attaching to composetest_redis_1, composetest_web_1
    redis_1 | [1] 26 Mar 02:58:52.356 # Server started, Redis version 2.8.19
    web_1   |  * Running on http://0.0.0.0:5000/

The web app should now be listening on port 5000 on your Docker daemon host (if
you're using Boot2docker, `boot2docker ip` will tell you its address).

If you want to run your services in the background, you can pass the `-d` flag
(for daemon mode) to `docker-compose up` and use `docker-compose ps` to see what
is currently running:

    $ docker-compose up -d
    Starting composetest_redis_1...
    Starting composetest_web_1...
    $ docker-compose ps
	         Name                     Command             State           Ports
    ----------------------------------------------------------------------------------
    composetest_redis_1   /entrypoint.sh redis-server   Up      6379/tcp
    composetest_web_1     python app.py                 Up      0.0.0.0:5000->5000/tcp

The `docker-compose run` command allows you to run one-off commands for your
services. For example, to see what environment variables are available to the
`web` service:

    $ docker-compose run web env

See `docker-compose --help` to see other available commands.

If you started Compose with `docker-compose up -d`, you'll probably want to stop
your services once you've finished with them:

    $ docker-compose stop

### Add a volume to your container

Suppose you made some changes to your web app by adding an extra route for your landing page.

```python
from flask import Flask
from redis import Redis
import os
app = Flask(__name__)
redis = Redis(host='redis', port=6379)

@app.route('/')
def index():
    return 'Index Page'

@app.route('/hello')
def hello():
    redis.incr('hits')
    return 'Hello World! I have been seen %s times.' % redis.get('hits')

if __name__ == "__main__":
    app.run(host="0.0.0.0", debug=True)
```

You will notice that the changes are not reflected in the browser.  Instead of running `docker-compose stop`
and `docker-compose up` to recreate the container every time you make a change, you can add a volume instead.
In order to mount the current directory inside the container so we can work on code without having to rebuild the image, modify `docker-compose.yml`:


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

Recreate the container and make some more changes. If you are not running in -d mode you will see:

    web_1   |  * Detected change in '/code/app.py', reloading
    web_1   |  * Restarting with stat

At this point, you have seen the basics of how Compose works.

- Next, try the quick start guide for [Django](django.md),
  [Rails](rails.md), or [Wordpress](wordpress.md).
- See the reference guides for complete details on the [commands](cli.md), the
  [configuration file](yml.md) and [environment variables](env.md).
