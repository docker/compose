Plum
====

**WARNING**: This is a work in progress and probably won't work yet. Feedback welcome.

Plum is tool for defining and running apps with Docker. It uses a simple, version-controllable YAML configuration file that looks something like this:

```yaml
db:
  image: orchardup/postgresql

web:
  build: web/
  link: db
```

Installing
----------

```bash
$ sudo pip install plum
```

Defining your app
-----------------

Put a `plum.yml` in your app's directory. Each top-level key defines a "service", such as a web app, database or cache. For each service, Plum will start a Docker container, so at minimum it needs to know what image to use.

The way to get started is to just give it an image name:

```yaml
db:
  image: orchardup/postgresql
```

Alternatively, you can give it the location of a directory with a Dockerfile (or a Git URL, as per the `docker build` command), and it'll automatically build the image for you:

```yaml
db:
  build: /path/to/postgresql/build/directory
```

You've now given Plum the minimal amount of configuration it needs to run:

```bash
$ plum up
Building db... done
db is running at 127.0.0.1:45678
<...output from postgresql server...>
```

For each service you've defined, Plum will start a Docker container with the specified image, building or pulling it if necessary. You now have a PostgreSQL server running at `127.0.0.1:45678`.

By default, `plum up` will run until each container has shut down, and relay their output to the terminal. To run in the background instead, pass the `-d` flag:

```bash
$ plum run -d
Building db... done
db is running at 127.0.0.1:45678

$ plum ps
SERVICE  STATE  PORT
db       up     45678
```


### Getting your code in

Some services may include your own code. To get that code into the container, ADD it in a Dockerfile.

`plum.yml`:

```yaml
web:
  build: web/
```

`web/Dockerfile`:

FROM orchardup/rails
ADD . /code
CMD: bundle exec rackup


### Communicating between containers

Your web app will probably need to talk to your database. You can use [Docker links] to enable containers to communicate, pass in the right IP address and port via environment variables:

```yaml
db:
  image: orchardup/postgresql

web:
  build: web/
  link: db
```

This will pass an environment variable called DB_PORT into the web container, whose value will look like `tcp://172.17.0.4:45678`. Your web app's code can then use that to connect to the database.

You can pass in multiple links, too:

```yaml
link:
 - db
 - memcached
 - redis
```


In each case, the resulting environment variable will begin with the uppercased name of the linked service (`DB_PORT`, `MEMCACHED_PORT`, `REDIS_PORT`).


### Container configuration options

You can pass extra configuration options to a container, much like with `docker run`:

```yaml
web:
  build: web/

-- override the default run command
run: bundle exec thin -p 3000

-- expose ports - can also be an array
ports: 3000

-- map volumes - can also be an array
volumes: /tmp/cache

-- add environment variables - can also be a dictionary
environment:
 - RACK_ENV=development
```


Running a one-off command
-------------------------

If you want to run a management command, use `plum run` to start a one-off container:

```bash
$ plum run db createdb myapp_development
$ plum run web rake db:migrate
$ plum run web bash
```


Running more than one container for a service
---------------------------------------------

You can set the number of containers to run for each service with `plum scale`:

```bash
$ plum up -d
db is running at 127.0.0.1:45678
web is running at 127.0.0.1:45679

$ plum scale db=0,web=3
Stopped db (127.0.0.1:45678)
Started web (127.0.0.1:45680)
Started web (127.0.0.1:45681)
```
