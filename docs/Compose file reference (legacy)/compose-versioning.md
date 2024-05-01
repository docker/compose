There are three legacy versions of the Compose file format:

- Version 1. This is specified by omitting a `version` key at the root of the YAML.

- Version 2.x. This is specified with a `version: '2'` or `version: '2.1'`, etc., entry at the root of the YAML.

- Version 3.x, designed to be cross-compatible between Compose and the Docker Engine's
[swarm mode](https://docs.docker.com/engine/swarm/). This is specified with a `version: '3'` or `version: '3.1'`, etc., entry at the root of the YAML.

The latest and recommended version of the Compose file format is defined by the [Compose Specification](https://docs.docker.com/compose/compose-file/). This format merges the 2.x and 3.x versions and is implemented by **Compose 1.27.0+**.


> **Note** 
>
> If you're using [multiple Compose files](https://docs.docker.com/compose/multiple-compose-files/) or
> [extending services](https://docs.docker.com/compose/multiple-compose-files/extends/),
> each file must be of the same version - you cannot, for example,
> mix version 1 and 2 in a single project.

Several things differ depending on which version you use:

- The structure and permitted configuration keys
- The minimum Docker Engine version you must be running
- Compose's behaviour with regards to networking

These differences are explained below.

### Version 2

Compose files using the version 2 syntax must indicate the version number at
the root of the document. All [services](compose-file-v2.md#service-configuration-reference)
must be declared under the `services` key.

Version 2 files are supported by **Compose 1.6.0+** and require a Docker Engine
of version **1.10.0+**.

Named [volumes](compose-file-v2.md#volume-configuration-reference) can be declared under the
`volumes` key, and [networks](compose-file-v2.md#network-configuration-reference) can be declared
under the `networks` key.

By default, every container joins an application-wide default network, and is
discoverable at a hostname that's the same as the service name. This means
[links](compose-file-v2.md#links) are largely unnecessary. For more details, see
[Networking in Compose](https://docs.docker.com/compose/networking/).

> **Note**
>
> With Compose version 2, when specifying the Compose file version to use, make sure to
> specify both the _major_ and _minor_ numbers. If no minor version is given,
> `0` is used by default and not the latest minor version. As a result, features added in later versions will not be supported. For example:
>
> ```yaml
> version: "2"
> ```
>
> is equivalent to:
>
> ```yaml
> version: "2.0"
> ```

Simple example:

    version: "{{% param "compose_file_v2" %}}"
    services:
      web:
        build: .
        ports:
         - "8000:5000"
        volumes:
         - .:/code
      redis:
        image: redis

A more extended example, defining volumes and networks:

    version: "{{% param "compose_file_v2" %}}"
    services:
      web:
        build: .
        ports:
         - "8000:5000"
        volumes:
         - .:/code
        networks:
          - front-tier
          - back-tier
      redis:
        image: redis
        volumes:
          - redis-data:/var/lib/redis
        networks:
          - back-tier
    volumes:
      redis-data:
        driver: local
    networks:
      front-tier:
        driver: bridge
      back-tier:
        driver: bridge

Several other options were added to support networking, such as:

* [`aliases`](compose-file-v2.md#aliases)

* The [`depends_on`](compose-file-v2.md#depends_on) option can be used in place of links to indicate dependencies
between services and startup order.

      version: "{{% param "compose_file_v2" %}}"
      services:
        web:
          build: .
          depends_on:
            - db
            - redis
        redis:
          image: redis
        db:
          image: postgres

* [`ipv4_address`, `ipv6_address`](compose-file-v2.md#ipv4_address-ipv6_address)

[Variable substitution](compose-file-v2.md#variable-substitution) also was added in Version 2.

### Version 2.1

An upgrade of [version 2](#version-2) that introduces new parameters only
available with Docker Engine version **1.12.0+**. Version 2.1 files are
supported by **Compose 1.9.0+**.

Introduces the following additional parameters:

- [`link_local_ips`](compose-file-v2.md#link_local_ips)
- [`isolation`](compose-file-v2.md#isolation-1) in build configurations and
  service definitions
- `labels` for [volumes](compose-file-v2.md#volume-configuration-reference),
  [networks](compose-file-v2.md#network-configuration-reference), and
  [build](compose-file-v3.md#build)
- `name` for [volumes](compose-file-v2.md#volume-configuration-reference)
- [`userns_mode`](compose-file-v2.md#userns_mode)
- [`healthcheck`](compose-file-v2.md#healthcheck)
- [`sysctls`](compose-file-v2.md#sysctls)
- [`pids_limit`](compose-file-v2.md#pids_limit)
- [`oom_kill_disable`](compose-file-v2.md#cpu-and-other-resources)
- [`cpu_period`](compose-file-v2.md#cpu-and-other-resources)

### Version 2.2

An upgrade of [version 2.1](#version-21) that introduces new parameters only
available with Docker Engine version **1.13.0+**.  Version 2.2 files are
supported by **Compose 1.13.0+**. This version also allows you to specify
default scale numbers inside the service's configuration.

Introduces the following additional parameters:

- [`init`](compose-file-v2.md#init)
- [`scale`](compose-file-v2.md#scale)
- [`cpu_rt_runtime` and `cpu_rt_period`](compose-file-v2.md#cpu_rt_runtime-cpu_rt_period)
- [`network`](compose-file-v2.md#network) for [build configurations](compose-file-v2.md#build)

### Version 2.3

An upgrade of [version 2.2](#version-22) that introduces new parameters only
available with Docker Engine version **17.06.0+**. Version 2.3 files are
supported by **Compose 1.16.0+**.

Introduces the following additional parameters:

- [`target`](compose-file-v2.md#target), [`extra_hosts`](compose-file-v2.md#extra_hosts-1) and
  [`shm_size`](compose-file-v2.md#shm_size) for [build configurations](compose-file-v2.md#build)
- `start_period` for [`healthchecks`](compose-file-v2.md#healthcheck)
- ["Long syntax" for volumes](compose-file-v2.md#long-syntax)
- [`runtime`](compose-file-v2.md#runtime) for service definitions
- [`device_cgroup_rules`](compose-file-v2.md#device_cgroup_rules)

### Version 2.4

An upgrade of [version 2.3](#version-23) that introduces new parameters only
available with Docker Engine version **17.12.0+**. Version 2.4 files are
supported by **Compose 1.21.0+**.

Introduces the following additional parameters:

- [`platform`](compose-file-v2.md#platform) for service definitions
- Support for extension fields at the root of service, network, and volume
  definitions

### Version 3

Designed to be cross-compatible between Compose and the Docker Engine's
[swarm mode](/engine/swarm/), version 3 removes several options and adds
several more.

- Removed: `volume_driver`, `volumes_from`, `cpu_shares`, `cpu_quota`,
  `cpuset`, `mem_limit`, `memswap_limit`, `extends`, `group_add`. See
  the [upgrading](#upgrading) guide for how to migrate away from these.

- Added: [deploy](compose-file-v3.md#deploy)

If only the major version is given (`version: '3'`),
the latest minor version is used by default.

### Version 3.1

An upgrade of [version 3](#version-3) that introduces new parameters only
available with Docker Engine version **1.13.1+**, and higher.

Introduces the following additional parameters:

- [`secrets`](compose-file-v3.md#secrets)

### Version 3.2

An upgrade of [version 3](#version-3) that introduces new parameters only
available with Docker Engine version **17.04.0+**, and higher.

Introduces the following additional parameters:

- [`cache_from`](compose-file-v3.md#cache_from) in [build configurations](compose-file-v3.md#build)
- Long syntax for [ports](compose-file-v3.md#ports) and [volume mounts](compose-file-v3.md#volumes)
- [`attachable`](compose-file-v3.md#attachable) network driver option
- [deploy `endpoint_mode`](compose-file-v3.md#endpoint_mode)
- [deploy placement `preference`](compose-file-v3.md#placement)

### Version 3.3

An upgrade of [version 3](#version-3) that introduces new parameters only
available with Docker Engine version **17.06.0+**, and higher.

Introduces the following additional parameters:

- [build `labels`](compose-file-v3.md#build)
- [`credential_spec`](compose-file-v3.md#credential_spec)
- [`configs`](compose-file-v3.md#configs)

### Version 3.4

An upgrade of [version 3](#version-3) that introduces new parameters. It is
only available with Docker Engine version **17.09.0** and higher.

Introduces the following additional parameters:

- [`target`](compose-file-v3.md#target) and [`network`](compose-file-v3.md#network) in
  [build configurations](compose-file-v3.md#build)
- `start_period` for [`healthchecks`](compose-file-v3.md#healthcheck)
- `order` for [update configurations](compose-file-v3.md#update_config)
- `name` for [volumes](compose-file-v3.md#volume-configuration-reference)

### Version 3.5

An upgrade of [version 3](#version-3) that introduces new parameters. It is
only available with Docker Engine version **17.12.0** and higher.

Introduces the following additional parameters:

- [`isolation`](compose-file-v3.md#isolation) in service definitions
- `name` for networks, secrets and configs
- `shm_size` in [build configurations](compose-file-v3.md#build)

### Version 3.6

An upgrade of [version 3](#version-3) that introduces new parameters. It is
only available with Docker Engine version **18.02.0** and higher.

Introduces the following additional parameters:

- [`tmpfs` size](compose-file-v3.md#long-syntax-3) for `tmpfs`-type mounts

### Version 3.7

An upgrade of [version 3](#version-3) that introduces new parameters. It is
only available with Docker Engine version **18.06.0** and higher.

Introduces the following additional parameters:

- [`init`](compose-file-v3.md#init) in service definitions
- [`rollback_config`](compose-file-v3.md#rollback_config) in deploy configurations
- Support for extension fields at the root of service, network, volume, secret
  and config definitions

### Version 3.8

An upgrade of [version 3](#version-3) that introduces new parameters. It is
only available with Docker Engine version **19.03.0** and higher.

Introduces the following additional parameters:

- [`max_replicas_per_node`](compose-file-v3.md#max_replicas_per_node) in placement
  configurations
- `template_driver` option for [config](compose-file-v3.md#configs-configuration-reference)
   and [secret](compose-file-v3.md#secrets-configuration-reference) configurations. This
   option is only supported when deploying swarm services using
   `docker stack deploy`.
- `driver` and `driver_opts` option for [secret](compose-file-v3.md#secrets-configuration-reference)
   configurations. This option is only supported when deploying swarm services
   using `docker stack deploy`.

### Version 1 (Deprecated)

Compose versions below 1.6.x are 

Compose files that do not declare a version are considered "version 1".  In those
files, all the [services](compose-file-v3.md#service-configuration-reference) are
declared at the root of the document.

Version 1 is supported by Compose up to 1.6.x** and has been deprecated.

Version 1 files cannot declare named
[volumes](compose-file-v3.md#volume-configuration-reference), [networks](compose-file-v3.md#network-configuration-reference) or
[build arguments](compose-file-v3.md#args).

Compose does not take advantage of [networking](https://docs.docker.com/compose/networking/) when you
use version 1: every container is placed on the default `bridge` network and is
reachable from every other container at its IP address. You need to use
`links` to enable discovery between containers.

Example:

    web:
      build: .
      ports:
       - "8000:5000"
      volumes:
       - .:/code
      links:
       - redis
    redis:
      image: redis

## Upgrading

### Version 2.x to 3.x

Between versions 2.x and 3.x, the structure of the Compose file is the same, but
several options have been removed:

-   `volume_driver`: Instead of setting the volume driver on the service, define
    a volume using the
    [top-level `volumes` option](compose-file-v3.md#volume-configuration-reference)
    and specify the driver there.

        version: "3.8"
        services:
          db:
            image: postgres
            volumes:
              - data:/var/lib/postgresql/data
        volumes:
          data:
            driver: mydriver

-   `volumes_from`: To share a volume between services, define it using the
    [top-level `volumes` option](compose-file-v3.md#volume-configuration-reference)
    and reference it from each service that shares it using the
    [service-level `volumes` option](compose-file-v3.md#driver).

-   `cpu_shares`, `cpu_quota`, `cpuset`, `mem_limit`, `memswap_limit`: These
    have been replaced by the [resources](compose-file-v3.md#resources) key under
    `deploy`. `deploy` configuration only takes effect when using
    `docker stack deploy`, and is ignored by `docker-compose`.

-   `extends`: This option has been removed for `version: "3.x"` Compose files.
    For more information on `extends`, see
    [Extending services](https://docs.docker.com/compose/multiple-compose-files/extends/).
-   `group_add`: This option has been removed for `version: "3.x"` Compose files.
-   `pids_limit`: This option has not been introduced in `version: "3.x"` Compose files.
-   `link_local_ips` in `networks`: This option has not been introduced in
    `version: "3.x"` Compose files.

#### Compatibility mode

`docker-compose` 1.20.0 introduces a new `--compatibility` flag designed to
help developers transition to version 3 more easily. When enabled,
`docker-compose` reads the `deploy` section of each service's definition and
attempts to translate it into the equivalent version 2 parameter. Currently,
the following deploy keys are translated:

- [resources](compose-file-v3.md#resources) limits and memory reservations
- [replicas](compose-file-v3.md#replicas)
- [restart_policy](compose-file-v3.md#restart_policy) `condition` and `max_attempts`

All other keys are ignored and produce a warning if present. You can review
the configuration that will be used to deploy by using the `--compatibility`
flag with the `config` command.

> Do not use this in production
>
> We recommend against using `--compatibility` mode in production. The
> resulting configuration is only an approximate using non-Swarm mode
> properties, it may produce unexpected results.

### Version 1 to 2.x

In the majority of cases, moving from version 1 to 2 is a very simple process:

1. Indent the whole file by one level and put a `services:` key at the top.
2. Add a `version: '2'` line at the top of the file.

It's more complicated if you're using particular configuration features:

-   `dockerfile`: This now lives under the `build` key:

        build:
          context: .
          dockerfile: Dockerfile-alternate

-   `log_driver`, `log_opt`: These now live under the `logging` key:

        logging:
          driver: syslog
          options:
            syslog-address: "tcp://192.168.0.42:123"

-   `links` with environment variables: environment variables created by
    links, such as `CONTAINERNAME_PORT`, ` have been deprecated for some time. In the new Docker network system,
    they have been removed. You should either connect directly to the
    appropriate hostname or set the relevant environment variable yourself,
    using the link hostname:

        web:
          links:
            - db
          environment:
            - DB_PORT=tcp://db:5432

-   `external_links`: Compose uses Docker networks when running version 2
    projects, so links behave slightly differently. In particular, two
    containers must be connected to at least one network in common in order to
    communicate, even if explicitly linked together.

    Either connect the external container to your app's
    [default network](https://docs.docker.com/compose/networking/), or connect both the external container and
    your service's containers to an
    [external network](https://docs.docker.com/compose/networking/).

-   `net`: This is now replaced by [network_mode](compose-file-v3.md#network_mode):

        net: host    ->  network_mode: host
        net: bridge  ->  network_mode: bridge
        net: none    ->  network_mode: none

    If you're using `net: "container:[service name]"`, you must now use
    `network_mode: "service:[service name]"` instead.

        net: "container:web"  ->  network_mode: "service:web"

    If you're using `net: "container:[container name/id]"`, the value does not
    need to change.

        net: "container:cont-name"  ->  network_mode: "container:cont-name"
        net: "container:abc12345"   ->  network_mode: "container:abc12345"

-   `volumes` with named volumes: these must now be explicitly declared in a
    top-level `volumes` section of your Compose file. If a service mounts a
    named volume called `data`, you must declare a `data` volume in your
    top-level `volumes` section. The whole file might look like this:

        version: "{{% param "compose_file_v2" %}}"
        services:
          db:
            image: postgres
            volumes:
              - data:/var/lib/postgresql/data
        volumes:
          data: {}

    By default, Compose creates a volume whose name is prefixed with your
    project name. If you want it to just be called `data`, declare it as
    external:

        volumes:
          data:
            external: true
