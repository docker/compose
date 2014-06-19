---
layout: default
title: fig.yml reference
---

fig.yml reference
=================

Each service defined in `fig.yml` must specify exactly one of `image` or `build`. Other keys are optional, and are analogous to their `docker run` command-line counterparts.

As with `docker run`, options specified in the Dockerfile (e.g. `CMD`, `EXPOSE`, `VOLUME`, `ENV`) are respected by default - you don't need to specify them again in `fig.yml`.

```yaml
-- Tag or partial image ID. Can be local or remote - Fig will attempt to pull
-- if it doesn't exist locally.
image: ubuntu
image: orchardup/postgresql
image: a4bc65fd

-- Path to a directory containing a Dockerfile. Fig will build and tag it with
-- a generated name, and use that image thereafter.
build: /path/to/build/dir

-- Override the default command.
command: bundle exec thin -p 3000

-- Link to containers in another service. Optionally specify an alternate name
-- for the link, which will determine how environment variables are prefixed,
-- e.g. "db" -> DB_1_PORT, "db:database" -> DATABASE_1_PORT
links:
 - db
 - db:database
 - redis

-- Expose ports. Either specify both ports (HOST:CONTAINER), or just the
-- container port (a random host port will be chosen).
-- Note: When mapping ports in the HOST:CONTAINER format, you may experience
-- erroneous results when using a container port lower than 60, because YAML
-- will parse numbers in the format "xx:yy" as sexagesimal (base 60). For
-- this reason, we recommend always explicitly specifying your port mappings
-- as strings.
ports:
 - "3000"
 - "8000:8000"
 - "49100:22"

-- Expose ports without publishing them to the host machine - they'll only be
-- accessible to linked services. Only the internal port can be specified.
expose:
 - "3000"
 - "8000"

-- Map volumes from the host machine (HOST:CONTAINER).
volumes:
 - cache/:/tmp/cache

-- Add environment variables.
environment:
  RACK_ENV: development
```

-- Networking mode. Use the same values as the docker client --net parameter
net: "host"
