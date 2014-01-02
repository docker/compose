Fig
====

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

    $ fig up
    Pulling image orchardup/postgresql...
    Building web...
    Starting example_db_1...
    Starting example_web_1...
    example_db_1 | 2014-01-02 14:47:18 UTC LOG:  database system is ready to accept connections
    example_web_1 |  * Running on http://0.0.0.0:5000/

There are commands to:

 - start, stop and rebuild services
 - view the status of running services
 - tail running services' log output
 - run a one-off command on a service

Fig is a project from [Orchard](https://orchardup.com), a Docker hosting service. [Follow us on Twitter](https://twitter.com/orchardup) to keep up to date with Fig and other Docker news.


Installing
----------

```bash
$ sudo pip install fig
```

Defining your app
-----------------

Put a `fig.yml` in your app's directory. Each top-level key defines a service, such as a web app, database or cache. For each service, Fig will start a Docker container, so at minimum it needs to know what image to use.

The simplest way to get started is to just give it an image name:

```yaml
db:
  image: orchardup/postgresql
```

You've now given Fig the minimal amount of configuration it needs to run:

```bash
$ fig up
Pulling image orchardup/postgresql...
Starting myapp_db_1...
myapp_db_1 is running at 127.0.0.1:45678
<...output from postgresql server...>
```

For each service you've defined, Fig will start a Docker container with the specified image, building or pulling it if necessary. You now have a PostgreSQL server running at `127.0.0.1:45678`.

By default, `fig up` will run until each container has shut down, and relay their output to the terminal. To run in the background instead, pass the `-d` flag:

```bash
$ fig up -d
Starting myapp_db_1... done
myapp_db_1 is running at 127.0.0.1:45678

$ fig ps
Name         State  Ports
------------------------------------
myapp_db_1   Up     5432->45678/tcp
```

### Building services

Fig can automatically build images for you if your service specifies a directory with a `Dockerfile` in it (or a Git URL, as per the `docker build` command).

This example will build an image with `app.py` inside it:

#### app.py

```python
print "Hello world!"
```

#### fig.yml

```yaml
web:
  build: .
```

#### Dockerfile

    FROM ubuntu:12.04
    RUN apt-get install python
    ADD . /opt
    WORKDIR /opt
    CMD python app.py



### Getting your code in

If you want to work on an application being run by Fig, you probably don't want to have to rebuild your image every time you make a change. To solve this, you can share the directory with the container using a volume so the changes are reflected immediately:

```yaml
web:
  build: .
  volumes:
   - .:/opt
```


### Communicating between containers

Your web app will probably need to talk to your database. You can use [Docker links] to enable containers to communicate, pass in the right IP address and port via environment variables:

```yaml
db:
  image: orchardup/postgresql

web:
  build: .
  links:
   - db
```

This will pass an environment variable called `MYAPP_DB_1_PORT` into the web container (where MYAPP is the name of the current directory). Your web app's code can use that to connect to the database.

```bash
$ fig up -d db
$ fig run web env
...
MYAPP_DB_1_PORT=tcp://172.17.0.5:5432
...
```

The full set of environment variables is documented in the Reference section.

Running a one-off command
-------------------------

If you want to run a management command, use `fig run` to start a one-off container:

```bash
$ fig run db createdb myapp_development
$ fig run web rake db:migrate
$ fig run web bash
```

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
