Change log
==========

1.4.2 (2015-09-22)
------------------

Fixes a regression in the 1.4.1 release that would cause `docker-compose up`
without the `-d` option to exit immediately.


1.4.1 (2015-09-10)
------------------

The following bugs have been fixed:

-   Some configuration changes (notably changes to `links`, `volumes_from`, and
    `net`) were not properly triggering a container recreate as part of
    `docker-compose up`.
-   `docker-compose up <service>` was showing logs for all services instead of
    just the specified services.
-   Containers with custom container names were showing up in logs as
    `service_number` instead of their custom container name.
-   When scaling a service sometimes containers would be recreated even when
    the configuration had not changed.


1.4.0 (2015-08-04)
------------------

-   By default, `docker-compose up` now only recreates containers for services whose configuration has changed since they were created. This should result in a dramatic speed-up for many applications.

    The experimental `--x-smart-recreate` flag which introduced this feature in Compose 1.3.0 has been removed, and a `--force-recreate` flag has been added for when you want to recreate everything.

-   Several of Compose's commands - `scale`, `stop`, `kill` and `rm` - now perform actions on multiple containers in parallel, rather than in sequence, which will run much faster on larger applications.

-   You can now specify a custom name for a service's container with `container_name`. Because Docker container names must be unique, this means you can't scale the service beyond one container.

-   You no longer have to specify a `file` option when using `extends` - it will default to the current file.

-   Service names can now contain dots, dashes and underscores.

-   Compose can now read YAML configuration from standard input, rather than from a file, by specifying `-` as the filename. This makes it easier to generate configuration dynamically:

        $ echo 'redis: {"image": "redis"}' | docker-compose --file - up

-   There's a new `docker-compose version` command which prints extended information about Compose's bundled dependencies.

-   `docker-compose.yml` now supports `log_opt` as well as `log_driver`, allowing you to pass extra configuration to a service's logging driver.

-   `docker-compose.yml` now supports `memswap_limit`, similar to `docker run --memory-swap`.

-   When mounting volumes with the `volumes` option, you can now pass in any mode supported by the daemon, not just `:ro` or `:rw`. For example, SELinux users can pass `:z` or `:Z`.

-   You can now specify a custom volume driver with the `volume_driver` option in `docker-compose.yml`, much like `docker run --volume-driver`.

-   A bug has been fixed where Compose would fail to pull images from private registries serving plain (unsecured) HTTP. The `--allow-insecure-ssl` flag, which was previously used to work around this issue, has been deprecated and now has no effect.

-   A bug has been fixed where `docker-compose build` would fail if the build depended on a private Hub image or an image from a private registry.

-   A bug has been fixed where Compose would crash if there were containers which the Docker daemon had not finished removing.

-   Two bugs have been fixed where Compose would sometimes fail with a "Duplicate bind mount" error, or fail to attach volumes to a container, if there was a volume path specified in `docker-compose.yml` with a trailing slash.

Thanks @mnowster, @dnephin, @ekristen, @funkyfuture, @jeffk and @lukemarsden!

1.3.3 (2015-07-15)
------------------

Two regressions have been fixed:

- When stopping containers gracefully, Compose was setting the timeout to 0, effectively forcing a SIGKILL every time.
- Compose would sometimes crash depending on the formatting of container data returned from the Docker API.

1.3.2 (2015-07-14)
------------------

The following bugs have been fixed:

- When there were one-off containers created by running `docker-compose run` on an older version of Compose, `docker-compose run` would fail with a name collision. Compose now shows an error if you have leftover containers of this type lying around, and tells you how to remove them.
- Compose was not reading Docker authentication config files created in the new location, `~/docker/config.json`, and authentication against private registries would therefore fail.
- When a container had a pseudo-TTY attached, its output in `docker-compose up` would be truncated.
- `docker-compose up --x-smart-recreate` would sometimes fail when an image tag was updated.
- `docker-compose up` would sometimes create two containers with the same numeric suffix.
- `docker-compose rm` and `docker-compose ps` would sometimes list services that aren't part of the current project (though no containers were erroneously removed).
- Some `docker-compose` commands would not show an error if invalid service names were passed in.

Thanks @dano, @josephpage, @kevinsimper, @lieryan, @phemmer, @soulrebel and @sschepens!

1.3.1 (2015-06-21)
------------------

The following bugs have been fixed:

- `docker-compose build` would always attempt to pull the base image before building.
- `docker-compose help migrate-to-labels` failed with an error.
- If no network mode was specified, Compose would set it to "bridge", rather than allowing the Docker daemon to use its configured default network mode.

1.3.0 (2015-06-18)
------------------

Firstly, two important notes:

- **This release contains breaking changes, and you will need to either remove or migrate your existing containers before running your app** - see the [upgrading section of the install docs](https://github.com/docker/compose/blob/1.3.0rc1/docs/install.md#upgrading) for details.

- Compose now requires Docker 1.6.0 or later.

We've done a lot of work in this release to remove hacks and make Compose more stable:

- Compose now uses container labels, rather than names, to keep track of containers. This makes Compose both faster and easier to integrate with your own tools.

- Compose no longer uses "intermediate containers" when recreating containers for a service. This makes `docker-compose up` less complex and more resilient to failure.

There are some new features:

- `docker-compose up` has an **experimental** new behaviour: it will only recreate containers for services whose configuration has changed in `docker-compose.yml`. This will eventually become the default, but for now you can take it for a spin:

        $ docker-compose up --x-smart-recreate

- When invoked in a subdirectory of a project, `docker-compose` will now climb up through parent directories until it finds a `docker-compose.yml`.

Several new configuration keys have been added to `docker-compose.yml`:

- `dockerfile`, like `docker build --file`, lets you specify an alternate Dockerfile to use with `build`.
- `labels`, like `docker run --labels`, lets you add custom metadata to containers.
- `extra_hosts`, like `docker run --add-host`, lets you add entries to a container's `/etc/hosts` file.
- `pid: host`, like `docker run --pid=host`, lets you reuse the same PID namespace as the host machine.
- `cpuset`, like `docker run --cpuset-cpus`, lets you specify which CPUs to allow execution in.
- `read_only`, like `docker run --read-only`, lets you mount a container's filesystem as read-only.
- `security_opt`, like `docker run --security-opt`, lets you specify [security options](https://docs.docker.com/reference/run/#security-configuration).
- `log_driver`, like `docker run --log-driver`, lets you specify a [log driver](https://docs.docker.com/reference/run/#logging-drivers-log-driver).

Many bugs have been fixed, including the following:

- The output of `docker-compose run` was sometimes truncated, especially when running under Jenkins.
- A service's volumes would sometimes not update after volume configuration was changed in `docker-compose.yml`.
- Authenticating against third-party registries would sometimes fail.
- `docker-compose run --rm` would fail to remove the container if the service had a `restart` policy in place.
- `docker-compose scale` would refuse to scale a service beyond 1 container if it exposed a specific port number on the host.
- Compose would refuse to create multiple volume entries with the same host path.

Thanks @ahromis, @albers, @aleksandr-vin, @antoineco, @ccverak, @chernjie, @dnephin, @edmorley, @fordhurley, @josephpage, @KyleJamesWalker, @lsowen, @mchasal, @noironetworks, @sdake, @sdurrheimer, @sherter, @stephenlawrence, @thaJeztah, @thieman, @turtlemonvh, @twhiteman, @vdemeester, @xuxinkun and @zwily!

1.2.0 (2015-04-16)
------------------

- `docker-compose.yml` now supports an `extends` option, which enables a service to inherit configuration from another service in another configuration file. This is really good for sharing common configuration between apps, or for configuring the same app for different environments. Here's the [documentation](https://github.com/docker/compose/blob/master/docs/yml.md#extends).

- When using Compose with a Swarm cluster, containers that depend on one another will be co-scheduled on the same node. This means that most Compose apps will now work out of the box, as long as they don't use `build`.

- Repeated invocations of `docker-compose up` when using Compose with a Swarm cluster now work reliably.

- Directories passed to `build`, filenames passed to `env_file` and volume host paths passed to `volumes` are now treated as relative to the *directory of the configuration file*, not the directory that `docker-compose` is being run in. In the majority of cases, those are the same, but if you use the `-f|--file` argument to specify a configuration file in another directory, **this is a breaking change**.

- A service can now share another service's network namespace with `net: container:<service>`.

- `volumes_from` and `net: container:<service>` entries are taken into account when resolving dependencies, so `docker-compose up <service>` will correctly start all dependencies of `<service>`.

- `docker-compose run` now accepts a `--user` argument to specify a user to run the command as, just like `docker run`.

- The `up`, `stop` and `restart` commands now accept a `--timeout` (or `-t`) argument to specify how long to wait when attempting to gracefully stop containers, just like `docker stop`.

- `docker-compose rm` now accepts `-f` as a shorthand for `--force`, just like `docker rm`.

Thanks, @abesto, @albers, @alunduil, @dnephin, @funkyfuture, @gilclark, @IanVS, @KingsleyKelly, @knutwalker, @thaJeztah and @vmalloc!

1.1.0 (2015-02-25)
------------------

Fig has been renamed to Docker Compose, or just Compose for short. This has several implications for you:

- The command you type is now `docker-compose`, not `fig`.
- You should rename your fig.yml to docker-compose.yml.
- If you’re installing via PyPi, the package is now `docker-compose`, so install it with `pip install docker-compose`.

Besides that, there’s a lot of new stuff in this release:

- We’ve made a few small changes to ensure that Compose will work with Swarm, Docker’s new clustering tool (https://github.com/docker/swarm). Eventually you'll be able to point Compose at a Swarm cluster instead of a standalone Docker host and it’ll run your containers on the cluster with no extra work from you. As Swarm is still developing, integration is rough and lots of Compose features don't work yet.

- `docker-compose run` now has a `--service-ports` flag for exposing ports on the given service. This is useful for e.g. running your webapp with an interactive debugger.

- You can now link to containers outside your app with the `external_links` option in docker-compose.yml.

- You can now prevent `docker-compose up` from automatically building images with the `--no-build` option. This will make fewer API calls and run faster.

- If you don’t specify a tag when using the `image` key, Compose will default to the `latest` tag, rather than pulling all tags.

- `docker-compose kill` now supports the `-s` flag, allowing you to specify the exact signal you want to send to a service’s containers.

- docker-compose.yml now has an `env_file` key, analogous to `docker run --env-file`, letting you specify multiple environment variables in a separate file. This is great if you have a lot of them, or if you want to keep sensitive information out of version control.

- docker-compose.yml now supports the `dns_search`, `cap_add`, `cap_drop`, `cpu_shares` and `restart` options, analogous to `docker run`’s `--dns-search`, `--cap-add`, `--cap-drop`, `--cpu-shares` and `--restart` options.

- Compose now ships with Bash tab completion - see the installation and usage docs at https://github.com/docker/compose/blob/1.1.0/docs/completion.md

- A number of bugs have been fixed - see the milestone for details: https://github.com/docker/compose/issues?q=milestone%3A1.1.0+

Thanks @dnephin, @squebe, @jbalonso, @raulcd, @benlangfield, @albers, @ggtools, @bersace, @dtenenba, @petercv, @drewkett, @TFenby, @paulRbr, @Aigeruth and @salehe!

1.0.1 (2014-11-04)
------------------

 - Added an `--allow-insecure-ssl` option to allow `fig up`, `fig run` and `fig pull` to pull from insecure registries.
 - Fixed `fig run` not showing output in Jenkins.
 - Fixed a bug where Fig couldn't build Dockerfiles with ADD statements pointing at URLs.

1.0.0 (2014-10-16)
------------------

The highlights:

 - [Fig has joined Docker.](https://www.orchardup.com/blog/orchard-is-joining-docker) Fig will continue to be maintained, but we'll also be incorporating the best bits of Fig into Docker itself.

   This means the GitHub repository has moved to [https://github.com/docker/fig](https://github.com/docker/fig) and our IRC channel is now #docker-fig on Freenode.

 - Fig can be used with the [official Docker OS X installer](https://docs.docker.com/installation/mac/). Boot2Docker will mount the home directory from your host machine so volumes work as expected.

 - Fig supports Docker 1.3.

 - It is now possible to connect to the Docker daemon using TLS by using the `DOCKER_CERT_PATH` and `DOCKER_TLS_VERIFY` environment variables.

 - There is a new `fig port` command which outputs the host port binding of a service, in a similar way to `docker port`.

 - There is a new `fig pull` command which pulls the latest images for a service.

 - There is a new `fig restart` command which restarts a service's containers.

 - Fig creates multiple containers in service by appending a number to the service name (e.g. `db_1`, `db_2`, etc). As a convenience, Fig will now give the first container an alias of the service name (e.g. `db`).
   
   This link alias is also a valid hostname and added to `/etc/hosts` so you can connect to linked services using their hostname. For example, instead of resolving the environment variables `DB_PORT_5432_TCP_ADDR` and `DB_PORT_5432_TCP_PORT`, you could just use the hostname `db` and port `5432` directly.

 - Volume definitions now support `ro` mode, expanding `~` and expanding environment variables.

 - `.dockerignore` is supported when building.

 - The project name can be set with the `FIG_PROJECT_NAME` environment variable.

 - The `--env` and `--entrypoint` options have been added to `fig run`.

 - The Fig binary for Linux is now linked against an older version of glibc so it works on CentOS 6 and Debian Wheezy.

Other things:

 - `fig ps` now works on Jenkins and makes fewer API calls to the Docker daemon.
 - `--verbose` displays more useful debugging output.
 - When starting a service where `volumes_from` points to a service without any containers running, that service will now be started.
 - Lots of docs improvements. Notably, environment variables are documented and official repositories are used throughout.

Thanks @dnephin, @d11wtq, @marksteve, @rubbish, @jbalonso, @timfreund, @alunduil, @mieciu, @shuron, @moss, @suzaku and @chmouel! Whew.

0.5.2 (2014-07-28)
------------------

 - Added a `--no-cache` option to `fig build`, which bypasses the cache just like `docker build --no-cache`.
 - Fixed the `dns:` fig.yml option, which was causing fig to error out.
 - Fixed a bug where fig couldn't start under Python 2.6.
 - Fixed a log-streaming bug that occasionally caused fig to exit.

Thanks @dnephin and @marksteve!


0.5.1 (2014-07-11)
------------------

 - If a service has a command defined, `fig run [service]` with no further arguments will run it.
 - The project name now defaults to the directory containing fig.yml, not the current working directory (if they're different)
 - `volumes_from` now works properly with containers as well as services
 - Fixed a race condition when recreating containers in `fig up`

Thanks @ryanbrainard and @d11wtq!


0.5.0 (2014-07-11)
------------------

 - Fig now starts links when you run `fig run` or `fig up`.
   
   For example, if you have a `web` service which depends on a `db` service, `fig run web ...` will start the `db` service.

 - Environment variables can now be resolved from the environment that Fig is running in. Just specify it as a blank variable in your `fig.yml` and, if set, it'll be resolved:
   ```
   environment:
     RACK_ENV: development
     SESSION_SECRET:
   ```

 - `volumes_from` is now supported in `fig.yml`. All of the volumes from the specified services and containers will be mounted:

   ```
   volumes_from:
    - service_name
    - container_name
   ```

 - A host address can now be specified in `ports`:

   ```
   ports:
    - "0.0.0.0:8000:8000"
    - "127.0.0.1:8001:8001"
   ```

 - The `net` and `workdir` options are now supported in `fig.yml`.
 - The `hostname` option now works in the same way as the Docker CLI, splitting out into a `domainname` option.
 - TTY behaviour is far more robust, and resizes are supported correctly.
 - Load YAML files safely.

Thanks to @d11wtq, @ryanbrainard, @rail44, @j0hnsmith, @binarin, @Elemecca, @mozz100 and @marksteve for their help with this release!


0.4.2 (2014-06-18)
------------------

 - Fix various encoding errors when using `fig run`, `fig up` and `fig build`.

0.4.1 (2014-05-08)
------------------

 - Add support for Docker 0.11.0. (Thanks @marksteve!)
 - Make project name configurable. (Thanks @jefmathiot!)
 - Return correct exit code from `fig run`.

0.4.0 (2014-04-29)
------------------

 - Support Docker 0.9 and 0.10
 - Display progress bars correctly when pulling images (no more ski slopes)
 - `fig up` now stops all services when any container exits
 - Added support for the `privileged` config option in fig.yml (thanks @kvz!)
 - Shortened and aligned log prefixes in `fig up` output
 - Only containers started with `fig run` link back to their own service
 - Handle UTF-8 correctly when streaming `fig build/run/up` output (thanks @mauvm and @shanejonas!)
 - Error message improvements

0.3.2 (2014-03-05)
------------------

 - Added an `--rm` option to `fig run`. (Thanks @marksteve!)
 - Added an `expose` option to `fig.yml`.

0.3.1 (2014-03-04)
------------------

 - Added contribution instructions. (Thanks @kvz!)
 - Fixed `fig rm` throwing an error.
 - Fixed a bug in `fig ps` on Docker 0.8.1 when there is a container with no command.

0.3.0 (2014-03-03)
------------------

 - We now ship binaries for OS X and Linux. No more having to install with Pip!
 - Add `-f` flag to specify alternate `fig.yml` files
 - Add support for custom link names
 - Fix a bug where recreating would sometimes hang
 - Update docker-py to support Docker 0.8.0.
 - Various documentation improvements
 - Various error message improvements

Thanks @marksteve, @Gazler and @teozkr!

0.2.2 (2014-02-17)
------------------

 - Resolve dependencies using Cormen/Tarjan topological sort
 - Fix `fig up` not printing log output
 - Stop containers in reverse order to starting
 - Fix scale command not binding ports

Thanks to @barnybug and @dustinlacewell for their work on this release.

0.2.1 (2014-02-04)
------------------

 - General improvements to error reporting (#77, #79)

0.2.0 (2014-01-31)
------------------

 - Link services to themselves so run commands can access the running service. (#67)
 - Much better documentation.
 - Make service dependency resolution more reliable. (#48)
 - Load Fig configurations with a `.yaml` extension. (#58)

Big thanks to @cameronmaske, @mrchrisadams and @damianmoore for their help with this release.

0.1.4 (2014-01-27)
------------------

 - Add a link alias without the project name. This makes the environment variables a little shorter: `REDIS_1_PORT_6379_TCP_ADDR`. (#54)

0.1.3 (2014-01-23)
------------------

 - Fix ports sometimes being configured incorrectly. (#46)
 - Fix log output sometimes not displaying. (#47)

0.1.2 (2014-01-22)
------------------

 - Add `-T` option to `fig run` to disable pseudo-TTY. (#34)
 - Fix `fig up` requiring the ubuntu image to be pulled to recreate containers. (#33) Thanks @cameronmaske!
 - Improve reliability, fix arrow keys and fix a race condition in `fig run`. (#34, #39, #40)

0.1.1 (2014-01-17)
------------------

 - Fix bug where ports were not exposed correctly (#29). Thanks @dustinlacewell!

0.1.0 (2014-01-16)
------------------

 - Containers are recreated on each `fig up`, ensuring config is up-to-date with `fig.yml` (#2)
 - Add `fig scale` command (#9)
 - Use `DOCKER_HOST` environment variable to find Docker daemon, for consistency with the official Docker client (was previously `DOCKER_URL`) (#19)
 - Truncate long commands in `fig ps` (#18)
 - Fill out CLI help banners for commands (#15, #16)
 - Show a friendlier error when `fig.yml` is missing (#4)
 - Fix bug with `fig build` logging (#3)
 - Fix bug where builds would time out if a step took a long time without generating output (#6)
 - Fix bug where streaming container output over the Unix socket raised an error (#7)

Big thanks to @tomstuart, @EnTeQuAk, @schickling, @aronasorman and @GeoffreyPlitt.

0.0.2 (2014-01-02)
------------------

 - Improve documentation
 - Try to connect to Docker on `tcp://localdocker:4243` and a UNIX socket in addition to `localhost`.
 - Improve `fig up` behaviour
 - Add confirmation prompt to `fig rm`
 - Add `fig build` command

0.0.1 (2013-12-20)
------------------

Initial release.


