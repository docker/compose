Fig
===

[![Build Status](https://travis-ci.org/orchardup/fig.png?branch=master)](https://travis-ci.org/orchardup/fig)

Punctual, lightweight development environments using Docker.

Fig is a tool for defining and running isolated application environments. You define the services which comprise your app in a simple, version-controllable YAML configuration file that looks like this:

```yaml
web:
  build: .
  links:
   - db
  ports:
   - 8000:8000
db:
  image: orchardup/postgresql
```

Then type `fig up`, and Fig will start and run your entire app:

![example fig run](https://orchardup.com/static/images/fig-example.5807d0d2dbe6.gif)

There are commands to:

 - start, stop and rebuild services
 - view the status of running services
 - tail running services' log output
 - run a one-off command on a service

Fig is a project from [Orchard](https://orchardup.com), a Docker hosting service. [Follow us on Twitter](https://twitter.com/orchardup) to keep up to date with Fig and other Docker news.


Getting started
---------------

Let's get a basic Python web app running on Fig. It assumes a little knowledge of Python, but the concepts should be clear if you're not familiar with it.

First, install Docker. If you're on OS X, you can use [docker-osx](https://github.com/noplay/docker-osx):

    $ curl https://raw.github.com/noplay/docker-osx/master/docker-osx > /usr/local/bin/docker-osx
    $ chmod +x /usr/local/bin/docker-osx
    $ docker-osx shell

Docker has guides for [Ubuntu](http://docs.docker.io/en/latest/installation/ubuntulinux/) and [other platforms](http://docs.docker.io/en/latest/installation/) in their documentation.

Next, install Fig:

    $ sudo pip install fig

(If you don’t have pip installed, try `brew install python` or `apt-get install python-pip`.)

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
    host=os.environ.get('FIGTEST_REDIS_1_PORT_6379_TCP_ADDR'),
    port=int(os.environ.get('FIGTEST_REDIS_1_PORT_6379_TCP_PORT'))
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

And we define how to build this into a Docker image using a file called `Dockerfile`:

    FROM stackbrew/ubuntu:13.10
    RUN apt-get -qq update
    RUN apt-get install -y python python-pip
    ADD . /code
    WORKDIR /code
    RUN pip install -r requirements.txt
    EXPOSE 5000
    CMD python app.py

That tells Docker to create an image with Python and Flask installed on it, run the command `python app.py`, and open port 5000 (the port that Flask listens on).

We then define a set of services using `fig.yml`:

    web:
      build: .
      ports:
       - 5000:5000
      volumes:
       - .:/code
      links:
       - redis
    redis:
      image: orchardup/redis

This defines two services:

 - `web`, which is built from `Dockerfile` in the current directory. It also says to forward the exposed port 5000 on the container to port 5000 on the host machine, connect up the Redis service, and mount the current directory inside the container so we can work on code without having to rebuild the image.
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

You'll probably want to stop your services when you've finished with them:

    $ fig stop

That's more-or-less how Fig works. See the reference section below for full details on the commands, configuration file and environment variables. If you have any thoughts or suggestions, [open an issue on GitHub](https://github.com/orchardup/fig) or [email us](mailto:hello@orchardup.com).


Reference
---------

### fig.yml

Each service defined in `fig.yml` must specify exactly one of `image` or `build`. Other keys are optional, and are analogous to their `docker run` command-line counterparts.

As with `docker run`, options specified in the Dockerfile (e.g. `CMD`, `EXPOSE`, `VOLUME`, `ENV`) are respected by default - you don't need to specify them again in `fig.yml`.

```yaml
-- Tag or partial image ID. Can be local or remote - Fig will attempt to pull if it doesn't exist locally.
image: ubuntu
image: orchardup/postgresql
image: a4bc65fd

-- Path to a directory containing a Dockerfile. Fig will build and tag it with a generated name, and use that image thereafter.
build: /path/to/build/dir

-- Override the default command.
command: bundle exec thin -p 3000

-- Link to containers in another service (see "Communicating between containers").
links:
 - db
 - redis

-- Expose ports. Either specify both ports (HOST:CONTAINER), or just the container port (a random host port will be chosen).
ports:
 - 3000
 - 8000:8000

-- Map volumes from the host machine (HOST:CONTAINER).
volumes:
 - cache/:/tmp/cache

-- Add environment variables.
environment:
  RACK_ENV: development
```

### Commands

Most commands are run against one or more services. If the service is omitted, it will apply to all services.

Run `fig [COMMAND] --help` for full usage.

#### build

Build or rebuild services.

Services are built once and then tagged as `project_service`. If you change a service's `Dockerfile` or its configuration in `fig.yml`, you will probably need to run `fig build` to rebuild it, then run `fig rm` to make `fig up` recreate your containers.

#### kill

Force stop service containers.

#### logs

View output from services.

#### ps

List running containers.

#### rm

Remove stopped service containers.


#### run

Run a one-off command for a service. E.g.:

    $ fig run web python manage.py shell

Note that this will not start any services that the command's service links to. So if, for example, your one-off command talks to your database, you will need to run `fig up -d db` first.

#### start

Start existing containers for a service.

#### stop

Stop running containers without removing them. They can be started again with `fig start`.

#### up

Build, create, start and attach to containers for a service. 

If there are stopped containers for a service, `fig up` will start those again instead of creating new containers. When it exits, the containers it started will be stopped. This means if you want to recreate containers, you will need to explicitly run `fig rm`.

### Environment variables

Fig uses [Docker links] to expose services' containers to one another. Each linked container injects a set of environment variables, each of which begins with the uppercase name of the container.

<b><i>name</i>\_PORT</b><br>
Full URL, e.g. `MYAPP_DB_1_PORT=tcp://172.17.0.5:5432`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i></b><br>
Full URL, e.g. `MYAPP_DB_1_PORT_5432_TCP=tcp://172.17.0.5:5432`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i>\_ADDR</b><br>
Container's IP address, e.g. `MYAPP_DB_1_PORT_5432_TCP_ADDR=172.17.0.5`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i>\_PORT</b><br>
Exposed port number, e.g. `MYAPP_DB_1_PORT_5432_TCP_PORT=5432`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i>\_PROTO</b><br>
Protocol (tcp or udp), e.g. `MYAPP_DB_1_PORT_5432_TCP_PROTO=tcp`

<b><i>name</i>\_NAME</b><br>
Fully qualified container name, e.g. `MYAPP_DB_1_NAME=/myapp_web_1/myapp_db_1`


[Docker links]: http://docs.docker.io/en/latest/use/port_redirection/#linking-a-container
