Plum
====

**WARNING**: This is a work in progress and probably won't work yet. Feedback welcome.

Plum is tool for defining and running application environments with Docker. It uses a simple, version-controllable YAML configuration file that looks something like this:

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

Installing
----------

```bash
$ sudo pip install plum
```

Defining your app
-----------------

Put a `plum.yml` in your app's directory. Each top-level key defines a "service", such as a web app, database or cache. For each service, Plum will start a Docker container, so at minimum it needs to know what image to use.

The simplest way to get started is to just give it an image name:

```yaml
db:
  image: orchardup/postgresql
```

You've now given Plum the minimal amount of configuration it needs to run:

```bash
$ plum start
Pulling image orchardup/postgresql...
Starting myapp_db_1...
myapp_db_1 is running at 127.0.0.1:45678
<...output from postgresql server...>
```

For each service you've defined, Plum will start a Docker container with the specified image, building or pulling it if necessary. You now have a PostgreSQL server running at `127.0.0.1:45678`.

By default, `plum start` will run until each container has shut down, and relay their output to the terminal. To run in the background instead, pass the `-d` flag:

```bash
$ plum start -d
Starting myapp_db_1... done
myapp_db_1 is running at 127.0.0.1:45678

$ plum ps
Name         State  Ports
------------------------------------
myapp_db_1   Up     5432->45678/tcp
```

### Building services

Plum can automatically build images for you if your service specifies a directory with a `Dockerfile` in it (or a Git URL, as per the `docker build` command).

This example will build an image with `app.py` inside it:

#### app.py

```python
print "Hello world!"
```

#### plum.yaml

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

If you want to work on an application being run by Plum, you probably don't want to have to rebuild your image every time you make a change. To solve this, you can share the directory with the container using a volume so the changes are reflected immediately:

```yaml
web:
  build: .
  volumes:
   - .:/opt
```


### Communicating between containers

Your dweb app will probably need to talk to your database. You can use [Docker links](http://docs.docker.io/en/latest/use/port_redirection/#linking-a-container) to enable containers to communicate, pass in the right IP address and port via environment variables:

```yaml
db:
  image: orchardup/postgresql

web:
  build: .
  links:
   - db
```

This will pass an environment variable called `MYAPP_DB_1_PORT` into the web container, whose value will look like `tcp://172.17.0.4:45678`. Your web app's code can use that to connect to the database. To see all of the environment variables available, run `env` inside a container:

```bash
$ plum start -d db
$ plum run web env
```


### Container configuration options

You can pass extra configuration options to a container, much like with `docker run`:

```yaml
web:
  build: .

  -- override the default command
  command: bundle exec thin -p 3000

  -- expose ports, optionally specifying both host and container ports (a random host port will be chosen otherwise)
  ports:
   - 3000
   - 8000:8000

  -- map volumes
  volumes:
   - cache/:/tmp/cache

  -- add environment variables
  environment:
   RACK_ENV: development
```


Running a one-off command
-------------------------

If you want to run a management command, use `plum run` to start a one-off container:

```bash
$ plum run db createdb myapp_development
$ plum run web rake db:migrate
$ plum run web bash
```


