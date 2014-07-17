---
layout: default
title: fig.yml reference
---

fig.yml reference
=================

Each service defined in `fig.yml` must specify exactly one of `image` or `build`. Other keys are optional, and are analogous to their `docker run` command-line counterparts.

As with `docker run`, options specified in the Dockerfile (e.g. `CMD`, `EXPOSE`, `VOLUME`, `ENV`) are respected by default - you don't need to specify them again in `fig.yml`.

###image

Tag or partial image ID. Can be local or remote - Fig will attempt to pull if it doesn't exist locally.

```
image: ubuntu
image: orchardup/postgresql
image: a4bc65fd
```

### build

Path to a directory containing a Dockerfile. Fig will build and tag it with a generated name, and use that image thereafter.

```
build: /path/to/build/dir
```

### command

Override the default command.

```
command: bundle exec thin -p 3000
```

### links


Link to containers in another service. Optionally specify an alternate name for the link, which will determine how environment variables are prefixed, e.g. `db` -> `DB_1_PORT`, `db:database` -> `DATABASE_1_PORT`

```
links:
 - db
 - db:database
 - redis
```

### ports

Expose ports. Either specify both ports (`HOST:CONTAINER`), or just the container port (a random host port will be chosen).

**Note:** When mapping ports in the `HOST:CONTAINER` format, you may experience erroneous results when using a container port lower than 60, because YAML will parse numbers in the format `xx:yy` as sexagesimal (base 60). For this reason, we recommend always explicitly specifying your port mappings as strings.

```
ports:
 - "3000"
 - "8000:8000"
 - "49100:22"
 - "127.0.0.1:8001:8001"
```

### expose

Expose ports without publishing them to the host machine - they'll only be accessible to linked services. Only the internal port can be specified.

```
expose:
 - "3000"
 - "8000"
```

### volumes

Mount paths as volumes, optionally specifying a path on the host machine (`HOST:CONTAINER`).

```
volumes:
 - /var/lib/mysql
 - cache/:/tmp/cache
```

### volumes_from

Mount all of the volumes from another service or container.

```
volumes_from:
 - service_name
 - container_name
```

### environment

Add environment variables. You can use either an array or a dictionary.

Environment variables with only a key are resolved to their values on the machine Fig is running on, which can be helpful for secret or host-specific values.

```
environment:
  RACK_ENV: development
  SESSION_SECRET:

environment:
  - RACK_ENV=development
  - SESSION_SECRET
```

### net

Networking mode. Use the same values as the docker client `--net` parameter.

```
net: "bridge"
net: "none"
net: "container:[name or id]"
net: "host"
```
