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

<a name="links"></a>
### links

Link to containers in another service. Either specify both the service name and the link alias (`SERVICE:ALIAS`), or just the service name (which will also be used for the alias).

```
links:
 - db
 - db:database
 - redis
```

An entry with the alias' name will be created in `/etc/hosts` inside containers for this service, e.g:

```
172.17.2.186  db
172.17.2.186  database
172.17.2.187  redis
```

Environment variables will also be created - see the [environment variable reference](env.html) for details.

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

Mount paths as volumes, optionally specifying a path on the host machine
(`HOST:CONTAINER`), or an access mode (`HOST:CONTAINER:ro`).

```
volumes:
 - /var/lib/mysql
 - cache/:/tmp/cache
 - ~/configs:/etc/configs/:ro
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

### dns

Custom DNS servers. Can be a single value or a list.

```
dns: 8.8.8.8
dns:
  - 8.8.8.8
  - 9.9.9.9
```

### cap_add, cap_drop

Add or drop container capabilities.
See `man 7 capabilities` for a full list.

```
cap_add:
  - ALL

cap_drop:
  - NET_ADMIN
  - SYS_ADMIN
```

### working\_dir, entrypoint, user, hostname, domainname, mem\_limit, privileged, restart

Each of these is a single value, analogous to its [docker run](https://docs.docker.com/reference/run/) counterpart.

```
working_dir: /code
entrypoint: /code/entrypoint.sh
user: postgresql

hostname: foo
domainname: foo.com

mem_limit: 1000000000
privileged: true

restart: always
```
