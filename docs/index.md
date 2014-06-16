---
layout: default
title: Fig | Fast, isolated development environments using Docker
---

<strong class="strapline">Fast, isolated development environments using Docker.</strong>

Define your app's environment with Docker so it can be reproduced anywhere:

    FROM orchardup/python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

Define the services that make up your app so they can be run together in an isolated environment:

```yaml
web:
  build: .
  command: python app.py
  links:
   - db
  ports:
   - "8000:8000"
db:
  image: orchardup/postgresql
```

(No more installing Postgres on your laptop!)

Then type `fig up`, and Fig will start and run your entire app:

![example fig run](https://orchardup.com/static/images/fig-example-large.gif)

There are commands to:

 - start, stop and rebuild services
 - view the status of running services
 - tail running services' log output
 - run a one-off command on a service


Quick start
-----------

Let's get a basic Python web app running on Fig. It assumes a little knowledge of Python, but the concepts should be clear if you're not familiar with it.

First, [install Docker and Fig](install.html).

You'll want to make a directory for the project:

    $ mkdir figtest
    $ cd figtest

Inside this directory, create `app.py`, a simple web app that uses the Flask framework and increments a value in Redis:

```python
from flask import Flask
from redis import Redis
import os
app = Flask(__name__)
redis = Redis(
    host=os.environ.get('REDIS_1_PORT_6379_TCP_ADDR'),
    port=int(os.environ.get('REDIS_1_PORT_6379_TCP_PORT'))
)

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

    FROM orchardup/python:2.7
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt

This tells Docker to install Python, our code and our Python dependencies inside a Docker image. For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

We then define a set of services using `fig.yml`:

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
      image: orchardup/redis

This defines two services:

 - `web`, which is built from `Dockerfile` in the current directory. It also says to run the command `python app.py` inside the image, forward the exposed port 5000 on the container to port 5000 on the host machine, connect up the Redis service, and mount the current directory inside the container so we can work on code without having to rebuild the image.
 - `redis`, which uses the public image [orchardup/redis](https://index.docker.io/u/orchardup/redis/). 

Now if we run `fig up`, it'll pull a Redis image, build an image for our own code, and start everything up:

    $ fig up
    Pulling image orchardup/redis...
    Building web...
    Starting figtest_redis_1...
    Starting figtest_web_1...
    figtest_redis_1 | [8] 02 Jan 18:43:35.576 # Server started, Redis version 2.8.3
    figtest_web_1 |  * Running on http://0.0.0.0:5000/

Open up [http://localhost:5000](http://localhost:5000) in your browser (or [http://localdocker:5000](http://localdocker:5000) if you're using [docker-osx](https://github.com/noplay/docker-osx)) and you should see it running!

If you want to run your services in the background, you can pass the `-d` flag to `fig up` and use `fig ps` to see what is currently running:

    $ fig up -d
    Starting figtest_redis_1...
    Starting figtest_web_1...
    $ fig ps
            Name                 Command            State       Ports
    -------------------------------------------------------------------
    figtest_redis_1   /usr/local/bin/run         Up
    figtest_web_1     /bin/sh -c python app.py   Up      5000->5000/tcp

`fig run` allows you to run one-off commands for your services. For example, to see what environment variables are available to the `web` service:

    $ fig run web env


See `fig --help` other commands that are available.

If you started Fig with `fig up -d`, you'll probably want to stop your services once you've finished with them:

    $ fig stop

That's more-or-less how Fig works. See the reference section below for full details on the commands, configuration file and environment variables. If you have any thoughts or suggestions, [open an issue on GitHub](https://github.com/orchardup/fig) or [email us](mailto:hello@orchardup.com).
