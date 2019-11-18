Change log
==========

1.25.0 (2019-11-18)
-------------------

### Features

- Set no-colors to true if CLICOLOR env variable is set to 0

- Add working dir, config files and env file in service labels

- Add dependencies for ARM build

- Add BuildKit support, use `DOCKER_BUILDKIT=1` and `COMPOSE_DOCKER_CLI_BUILD=1`

- Bump paramiko to 2.6.0

- Add working dir, config files and env file in service labels

- Add tag `docker-compose:latest`

- Add `docker-compose:<version>-alpine` image/tag

- Add `docker-compose:<version>-debian` image/tag

- Bumped `docker-py` 4.1.0

- Supports `requests` up to 2.22.0 version

- Drops empty tag on `build:cache_from`

- `Dockerfile` now generates `libmusl` binaries for alpine

- Only pull images that can't be built

- Attribute `scale` can now accept `0` as a value

- Added `--quiet` build flag

- Added `--no-interpolate` to `docker-compose config`

- Bump OpenSSL for macOS build (`1.1.0j` to `1.1.1c`)

- Added `--no-rm` to `build` command

- Added support for `credential_spec`

- Resolve digests without pulling image

- Upgrade `pyyaml` to `4.2b1`

- Lowered severity to `warning` if `down` tries to remove nonexisting image

- Use improved API fields for project events when possible

- Update `setup.py` for modern `pypi/setuptools` and remove `pandoc` dependencies

- Removed `Dockerfile.armhf` which is no longer needed

### Bugfixes

- Make container service color deterministic, remove red from chosen colors

- Fix non ascii chars error. Python2 only

- Format image size as decimal to be align with Docker CLI

- Use Python Posix support to get tty size

- Fix same file 'extends' optimization

- Use python POSIX support to get tty size

- Format image size as decimal to be align with Docker CLI

- Fixed stdin_open

- Fixed `--remove-orphans` when used with `up --no-start`

- Fixed `docker-compose ps --all`

- Fixed `depends_on` dependency recreation behavior

- Fixed bash completion for `build --memory`

- Fixed misleading warning concerning env vars when performing an `exec` command

- Fixed failure check in parallel_execute_watch

- Fixed race condition after pulling image

- Fixed error on duplicate mount points

- Fixed merge on networks section

- Always connect Compose container to `stdin`

- Fixed the presentation of failed services on 'docker-compose start' when containers are not available

1.24.1 (2019-06-24)
-------------------

### Bugfixes

- Fixed acceptance tests

1.24.0 (2019-03-28)
-------------------

### Features

- Added support for connecting to the Docker Engine using the `ssh` protocol.

- Added a `--all` flag to `docker-compose ps` to include stopped one-off containers
  in the command's output.

- Add bash completion for `ps --all|-a`

- Support for credential_spec

- Add `--parallel` to `docker build`'s options in `bash` and `zsh` completion

### Bugfixes

- Fixed a bug where some valid credential helpers weren't properly handled by Compose
  when attempting to pull images from private registries.

- Fixed an issue where the output of `docker-compose start` before containers were created
  was misleading

- To match the Docker CLI behavior and to avoid confusing issues, Compose will no longer
  accept whitespace in variable names sourced from environment files.

- Compose will now report a configuration error if a service attempts to declare
  duplicate mount points in the volumes section.

- Fixed an issue with the containerized version of Compose that prevented users from
  writing to stdin during interactive sessions started by `run` or `exec`.

- One-off containers started by `run` no longer adopt the restart policy of the service,
  and are instead set to never restart.

- Fixed an issue that caused some container events to not appear in the output of
  the `docker-compose events` command.

- Missing images will no longer stop the execution of `docker-compose down` commands
  (a warning will be displayed instead).

- Force `virtualenv` version for macOS CI

- Fix merging of compose files when network has `None` config

- Fix `CTRL+C` issues by enabling `bootloader_ignore_signals` in `pyinstaller`

- Bump `docker-py` version to `3.7.2` to fix SSH and proxy config issues

- Fix release script and some typos on release documentation

1.23.2 (2018-11-28)
-------------------

### Bugfixes

- Reverted a 1.23.0 change that appended random strings to container names
  created by `docker-compose up`, causing addressability issues.
  Note: Containers created by `docker-compose run` will continue to use
  randomly generated names to avoid collisions during parallel runs.

- Fixed an issue where some `dockerfile` paths would fail unexpectedly when
  attempting to build on Windows.

- Fixed a bug where build context URLs would fail to build on Windows.

- Fixed a bug that caused `run` and `exec` commands to fail for some otherwise
  accepted values of the `--host` parameter.

- Fixed an issue where overrides for the `storage_opt` and `isolation` keys in
  service definitions weren't properly applied.

- Fixed a bug where some invalid Compose files would raise an uncaught
  exception during validation.

1.23.1 (2018-11-01)
-------------------

### Bugfixes

- Fixed a bug where working with containers created with a previous (< 1.23.0)
  version of Compose would cause unexpected crashes

- Fixed an issue where the behavior of the `--project-directory` flag would
  vary depending on which subcommand was being used.

1.23.0 (2018-10-30)
-------------------

### Important note

The default naming scheme for containers created by Compose in this version
has changed from `<project>_<service>_<index>` to
`<project>_<service>_<index>_<slug>`, where `<slug>` is a randomly-generated
hexadecimal string. Please make sure to update scripts relying on the old
naming scheme accordingly before upgrading.

### Features

- Logs for containers restarting after a crash will now appear in the output
  of the `up` and `logs` commands.

- Added `--hash` option to the `docker-compose config` command, allowing users
  to print a hash string for each service's configuration to facilitate rolling
  updates.

- Added `--parallel` flag to the `docker-compose build` command, allowing
  Compose to build up to 5 images simultaneously.

- Output for the `pull` command now reports status / progress even when pulling
  multiple images in parallel.

- For images with multiple names, Compose will now attempt to match the one
  present in the service configuration in the output of the `images` command.

### Bugfixes

- Parallel `run` commands for the same service will no longer fail due to name
  collisions.

- Fixed an issue where paths longer than 260 characters on Windows clients would
  cause `docker-compose build` to fail.

- Fixed a bug where attempting to mount `/var/run/docker.sock` with
  Docker Desktop for Windows would result in failure.

- The `--project-directory` option is now used by Compose to determine where to
  look for the `.env` file.

- `docker-compose build` no longer fails when attempting to pull an image with
  credentials provided by the gcloud credential helper.

- Fixed the `--exit-code-from` option in `docker-compose up` to always report
  the actual exit code even when the watched container isn't the cause of the
  exit.

- Fixed an issue that would prevent recreating a service in some cases where
  a volume would be mapped to the same mountpoint as a volume declared inside
  the image's Dockerfile.

- Fixed a bug that caused hash configuration with multiple networks to be
  inconsistent, causing some services to be unnecessarily restarted.

- Fixed a bug that would cause failures with variable substitution for services
  with a name containing one or more dot characters

- Fixed a pipe handling issue when using the containerized version of Compose.

- Fixed a bug causing `external: false` entries in the Compose file to be
  printed as `external: true` in the output of `docker-compose config`

- Fixed a bug where issuing a `docker-compose pull` command on services
  without a defined image key would cause Compose to crash

- Volumes and binds are now mounted in the order they're declared in the
  service definition

### Miscellaneous

- The `zsh` completion script has been updated with new options, and no
  longer suggests container names where service names are expected.

1.22.0 (2018-07-17)
-------------------

### Features

#### Compose format version 3.7

- Introduced version 3.7 of the `docker-compose.yml` specification.
  This version requires Docker Engine 18.06.0 or above.

- Added support for `rollback_config` in the deploy configuration

- Added support for the `init` parameter in service configurations

- Added support for extension fields in service, network, volume, secret,
  and config configurations

#### Compose format version 2.4

- Added support for extension fields in service, network,
  and volume configurations

### Bugfixes

- Fixed a bug that prevented deployment with some Compose files when
  `DOCKER_DEFAULT_PLATFORM` was set

- Compose will no longer try to create containers or volumes with
  invalid starting characters

- Fixed several bugs that prevented Compose commands from working properly
  with containers created with an older version of Compose

- Fixed an issue with the output of `docker-compose config` with the
  `--compatibility-mode` flag enabled when the source file contains
  attachable networks

- Fixed a bug that prevented the `gcloud` credential store from working
  properly when used with the Compose binary on UNIX

- Fixed a bug that caused connection errors when trying to operate
  over a non-HTTPS TCP connection on Windows

- Fixed a bug that caused builds to fail on Windows if the Dockerfile
  was located in a subdirectory of the build context

- Fixed an issue that prevented proper parsing of UTF-8 BOM encoded
  Compose files on Windows

- Fixed an issue with handling of the double-wildcard (`**`) pattern in `.dockerignore` files when using `docker-compose build`

- Fixed a bug that caused auth values in legacy `.dockercfg` files to be ignored
- `docker-compose build` will no longer attempt to create image names starting with an invalid character

1.21.2 (2018-05-03)
-------------------

### Bugfixes

- Fixed a bug where the ip_range attribute in IPAM configs was prevented
  from passing validation

1.21.1 (2018-04-27)
-------------------

### Bugfixes

- In 1.21.0, we introduced a change to how project names are sanitized for
  internal use in resource names. This caused issues when manipulating an
  existing, deployed application whose name had changed as a result.
  This release properly detects resources using "legacy" naming conventions.

- Fixed an issue where specifying an in-context Dockerfile using an absolute
  path would fail despite being valid.

- Fixed a bug where IPAM option changes were incorrectly detected, preventing
  redeployments.

- Validation of v2 files now properly checks the structure of IPAM configs.

- Improved support for credentials stores on Windows to include binaries using
  extensions other than `.exe`. The list of valid extensions is determined by
  the contents of the `PATHEXT` environment variable.

- Fixed a bug where Compose would generate invalid binds containing duplicate
  elements with some v3.2 files, triggering errors at the Engine level during
  deployment.

1.21.0 (2018-04-10)
-------------------

### New features

#### Compose file version 2.4

- Introduced version 2.4 of the `docker-compose.yml` specification.
  This version requires Docker Engine 17.12.0 or above.

- Added support for the `platform` parameter in service definitions.
  If supplied, the parameter is also used when performing build for the
  service.

#### Compose file version 2.2 and up

- Added support for the `cpu_rt_period` and `cpu_rt_runtime` parameters
  in service definitions (2.x only).

#### Compose file version 2.1 and up

- Added support for the `cpu_period` parameter in service definitions
  (2.x only).

- Added support for the `isolation` parameter in service build configurations.
  Additionally, the `isolation` parameter is used for builds as well if no
  `build.isolation` parameter is defined. (2.x only)

#### All formats

- Added support for the `--workdir` flag in `docker-compose exec`.

- Added support for the `--compress` flag in `docker-compose build`.

- `docker-compose pull` is now performed in parallel by default. You can
  opt out using the `--no-parallel` flag. The `--parallel` flag is now
  deprecated and will be removed in a future version.

- Dashes and underscores in project names are no longer stripped out.

- `docker-compose build` now supports the use of Dockerfile from outside
  the build context.

### Bugfixes

- Compose now checks that the volume's configuration matches the remote
  volume, and errors out if a mismatch is detected.

- Fixed a bug that caused Compose to raise unexpected errors when attempting
  to create several one-off containers in parallel.

- Fixed a bug with argument parsing when using `docker-machine config` to
  generate TLS flags for `exec` and `run` commands.

- Fixed a bug where variable substitution with an empty default value
  (e.g. `${VAR:-}`) would print an incorrect warning.

- Improved resilience when encoding of the Compose file doesn't match the
  system's. Users are encouraged to use UTF-8 when possible.

- Fixed a bug where external overlay networks in Swarm would be incorrectly
  recognized as inexistent by Compose, interrupting otherwise valid
  operations.

1.20.1 (2018-03-21)
-------------------

### Bugfixes

- Fixed an issue where `docker-compose build` would error out if the
  build context contained directory symlinks

1.20.0 (2018-03-20)
-------------------

### New features

#### Compose file version 3.6

- Introduced version 3.6 of the `docker-compose.yml` specification.
  This version requires Docker Engine 18.02.0 or above.

- Added support for the `tmpfs.size` property in volume mappings

#### Compose file version 3.2 and up

- The `--build-arg` option can now be used without specifying a service
  in `docker-compose build`

#### Compose file version 2.3

- Added support for `device_cgroup_rules` in service definitions

- Added support for the `tmpfs.size` property in long-form volume mappings

- The `--build-arg` option can now be used without specifying a service
  in `docker-compose build`

#### All formats

- Added a `--log-level` option to the top-level `docker-compose` command.
  Accepted values are `debug`, `info`, `warning`, `error`, `critical`.
  Default log level is `info`

- `docker-compose run` now allows users to unset the container's entrypoint

- Proxy configuration found in the `~/.docker/config.json` file now populates
  environment and build args for containers created by Compose

- Added the `--use-aliases` flag to `docker-compose run`, indicating that
  network aliases declared in the service's config should be used for the
  running container

- Added the `--include-deps` flag to `docker-compose pull`

- `docker-compose run` now kills and removes the running container upon
  receiving `SIGHUP`

- `docker-compose ps` now shows the containers' health status if available

- Added the long-form `--detach` option to the `exec`, `run` and `up`
  commands

### Bugfixes

- Fixed `.dockerignore` handling, notably with regard to absolute paths
  and last-line precedence rules

- Fixed an issue where Compose would make costly DNS lookups when connecting
  to the Engine when using Docker For Mac

- Fixed a bug introduced in 1.19.0 which caused the default certificate path
  to not be honored by Compose

- Fixed a bug where Compose would incorrectly check whether a symlink's
  destination was accessible when part of a build context

- Fixed a bug where `.dockerignore` files containing lines of whitespace
  caused Compose to error out on Windows

- Fixed a bug where `--tls*` and `--host` options wouldn't be properly honored
  for interactive `run` and `exec` commands

- A `seccomp:<filepath>` entry in the `security_opt` config now correctly
  sends the contents of the file to the engine

- ANSI output for `up` and `down` operations should no longer affect the wrong
  lines

- Improved support for non-unicode locales

- Fixed a crash occurring on Windows when the user's home directory name
  contained non-ASCII characters

- Fixed a bug occurring during builds caused by files with a negative `mtime`
  values in the build context

- Fixed an encoding bug when streaming build progress

1.19.0 (2018-02-07)
-------------------

### Breaking changes

- On UNIX platforms, interactive `run` and `exec` commands now require
  the `docker` CLI to be installed on the client by default. To revert
  to the previous behavior, users may set the `COMPOSE_INTERACTIVE_NO_CLI`
  environment variable.

### New features

#### Compose file version 3.x

- The output of the `config` command should now merge `deploy` options from
  several Compose files in a more accurate manner

#### Compose file version 2.3

- Added support for the `runtime` option in service definitions

#### Compose file version 2.1 and up

- Added support for the `${VAR:?err}` and `${VAR?err}` variable interpolation
  syntax to indicate mandatory variables

#### Compose file version 2.x

- Added `priority` key to service network mappings, allowing the user to
  define in which order the specified service will connect to each network

#### All formats

- Added `--renew-anon-volumes` (shorthand `-V`) to the `up` command,
  preventing Compose from recovering volume data from previous containers for
  anonymous volumes

- Added limit for number of simultaneous parallel operations, which should
  prevent accidental resource exhaustion of the server. Default is 64 and
  can be configured using the `COMPOSE_PARALLEL_LIMIT` environment variable

- Added `--always-recreate-deps` flag to the `up` command to force recreating
  dependent services along with the dependency owner

- Added `COMPOSE_IGNORE_ORPHANS` environment variable to forgo orphan
  container detection and suppress warnings

- Added `COMPOSE_FORCE_WINDOWS_HOST` environment variable to force Compose
  to parse volume definitions as if the Docker host was a Windows system,
  even if Compose itself is currently running on UNIX

- Bash completion should now be able to better differentiate between running,
  stopped and paused services

### Bugfixes

- Fixed a bug that would cause the `build` command to report a connection
  error when the build context contained unreadable files or FIFO objects.
  These file types will now be handled appropriately

- Fixed various issues around interactive `run`/`exec` sessions.

- Fixed a bug where setting TLS options with environment and CLI flags
  simultaneously would result in part of the configuration being ignored

- Fixed a bug where the DOCKER_TLS_VERIFY environment variable was being
  ignored by Compose

- Fixed a bug where the `-d` and `--timeout` flags in `up` were erroneously
  marked as incompatible

- Fixed a bug where the recreation of a service would break if the image
  associated with the previous container had been removed

- Fixed a bug where updating a mount's target would break Compose when
  trying to recreate the associated service

- Fixed a bug where `tmpfs` volumes declared using the extended syntax in
  Compose files using version 3.2 would be erroneously created as anonymous
  volumes instead

- Fixed a bug where type conversion errors would print a stacktrace instead
  of exiting gracefully

- Fixed some errors related to unicode handling

- Dependent services no longer get recreated along with the dependency owner
  if their configuration hasn't changed

- Added better validation of `labels` fields in Compose files. Label values
  containing scalar types (number, boolean) now get automatically converted
  to strings

1.18.0 (2017-12-15)
-------------------

### New features

#### Compose file version 3.5

- Introduced version 3.5 of the `docker-compose.yml` specification.
  This version requires Docker Engine 17.06.0 or above

- Added support for the `shm_size` parameter in build configurations

- Added support for the `isolation` parameter in service definitions

- Added support for custom names for network, secret and config definitions

#### Compose file version 2.3

- Added support for `extra_hosts` in build configuration

- Added support for the [long syntax](https://docs.docker.com/compose/compose-file/#long-syntax-3) for volume entries, as previously introduced in the 3.2 format.
  Note that using this syntax will create [mounts](https://docs.docker.com/engine/admin/volumes/bind-mounts/) instead of volumes.

#### Compose file version 2.1 and up

- Added support for the `oom_kill_disable` parameter in service definitions
  (2.x only)

- Added support for custom names for network definitions (2.x only)


#### All formats

- Values interpolated from the environment will now be converted to the
  proper type when used in non-string fields.

- Added support for `--label` in `docker-compose run`

- Added support for `--timeout` in `docker-compose down`

- Added support for `--memory` in `docker-compose build`

- Setting `stop_grace_period` in service definitions now also sets the
  container's `stop_timeout`

### Bugfixes

- Fixed an issue where Compose was still handling service hostname according
  to legacy engine behavior, causing hostnames containing dots to be cut up

- Fixed a bug where the `X-Y:Z` syntax for ports was considered invalid
  by Compose

- Fixed an issue with CLI logging causing duplicate messages and inelegant
  output to occur

- Fixed an issue that caused `stop_grace_period` to be ignored when using
  multiple Compose files

- Fixed a bug that caused `docker-compose images` to crash when using
  untagged images

- Fixed a bug where the valid `${VAR:-}` syntax would cause Compose to
  error out

- Fixed a bug where `env_file` entries using an UTF-8 BOM were being read
  incorrectly

- Fixed a bug where missing secret files would generate an empty directory
  in their place

- Fixed character encoding issues in the CLI's error handlers

- Added validation for the `test` field in healthchecks

- Added validation for the `subnet` field in IPAM configurations

- Added validation for `volumes` properties when using the long syntax in
  service definitions

- The CLI now explicit prevents using `-d` and `--timeout` together
  in `docker-compose up`

1.17.1 (2017-11-08)
------------------

### Bugfixes

- Fixed a bug that would prevent creating new containers when using
  container labels in the list format as part of the service's definition.

1.17.0 (2017-11-02)
-------------------

### New features

#### Compose file version 3.4

- Introduced version 3.4 of the `docker-compose.yml` specification.
  This version requires to be used with Docker Engine 17.06.0 or above.

- Added support for `cache_from`, `network` and `target` options in build
  configurations

- Added support for the `order` parameter in the `update_config` section

- Added support for setting a custom name in volume definitions using
  the `name` parameter

#### Compose file version 2.3

- Added support for `shm_size` option in build configuration

#### Compose file version 2.x

- Added support for extension fields (`x-*`). Also available for v3.4 files

#### All formats

- Added new `--no-start` to the `up` command, allowing users to create all
  resources (networks, volumes, containers) without starting services.
  The `create` command is deprecated in favor of this new option

### Bugfixes

- Fixed a bug where `extra_hosts` values would be overridden by extension
  files instead of merging together

- Fixed a bug where the validation for v3.2 files would prevent using the
  `consistency` field in service volume definitions

- Fixed a bug that would cause a crash when configuration fields expecting
  unique items would contain duplicates

- Fixed a bug where mount overrides with a different mode would create a
  duplicate entry instead of overriding the original entry

- Fixed a bug where build labels declared as a list wouldn't be properly
  parsed

- Fixed a bug where the output of `docker-compose config` would be invalid
  for some versions if the file contained custom-named external volumes

- Improved error handling when issuing a build command on Windows using an
  unsupported file version

- Fixed an issue where networks with identical names would sometimes be
  created when running `up` commands concurrently.

1.16.1 (2017-09-01)
-------------------

### Bugfixes

- Fixed bug that prevented using `extra_hosts` in several configuration files.

1.16.0 (2017-08-31)
-------------------

### New features

#### Compose file version 2.3

- Introduced version 2.3 of the `docker-compose.yml` specification.
  This version requires to be used with Docker Engine 17.06.0 or above.

- Added support for the `target` parameter in build configurations

- Added support for the `start_period` parameter in healthcheck
  configurations

#### Compose file version 2.x

- Added support for the `blkio_config` parameter in service definitions

- Added support for setting a custom name in volume definitions using
  the `name` parameter (not available for version 2.0)

#### All formats

- Added new CLI flag `--no-ansi` to suppress ANSI control characters in
  output

### Bugfixes

- Fixed a bug where nested `extends` instructions weren't resolved
  properly, causing "file not found" errors

- Fixed several issues with `.dockerignore` parsing

- Fixed issues where logs of TTY-enabled services were being printed
  incorrectly and causing `MemoryError` exceptions

- Fixed a bug where printing application logs would sometimes be interrupted
  by a `UnicodeEncodeError` exception on Python 3

- The `$` character in the output of `docker-compose config` is now
  properly escaped

- Fixed a bug where running `docker-compose top` would sometimes fail
  with an uncaught exception

- Fixed a bug where `docker-compose pull` with the `--parallel` flag
  would return a `0` exit code when failing

- Fixed an issue where keys in `deploy.resources` were not being validated

- Fixed an issue where the `logging` options in the output of
  `docker-compose config` would be set to `null`, an invalid value

- Fixed the output of the `docker-compose images` command when an image
  would come from a private repository using an explicit port number

- Fixed the output of `docker-compose config` when a port definition used
  `0` as the value for the published port

1.15.0 (2017-07-26)
-------------------

### New features

#### Compose file version 2.2

- Added support for the `network` parameter in build configurations.

#### Compose file version 2.1 and up

- The `pid` option in a service's definition now supports a `service:<name>`
  value.

- Added support for the `storage_opt` parameter in in service definitions.
  This option is not available for the v3 format

#### All formats

- Added `--quiet` flag to `docker-compose pull`, suppressing progress output

- Some improvements to CLI output

### Bugfixes

- Volumes specified through the `--volume` flag of `docker-compose run` now
  complement volumes declared in the service's definition instead of replacing
  them

- Fixed a bug where using multiple Compose files would unset the scale value
  defined inside the Compose file.

- Fixed an issue where the `credHelpers` entries in the `config.json` file
  were not being honored by Compose

- Fixed a bug where using multiple Compose files with port declarations
  would cause failures in Python 3 environments

- Fixed a bug where some proxy-related options present in the user's
  environment would prevent Compose from running

- Fixed an issue where the output of `docker-compose config` would be invalid
  if the original file used `Y` or `N` values

- Fixed an issue preventing `up` operations on a previously created stack on
  Windows Engine.

1.14.0 (2017-06-19)
-------------------

### New features

#### Compose file version 3.3

- Introduced version 3.3 of the `docker-compose.yml` specification.
  This version requires to be used with Docker Engine 17.06.0 or above.
  Note: the `credential_spec` and `configs` keys only apply to Swarm services
  and will be ignored by Compose

#### Compose file version 2.2

- Added the following parameters in service definitions: `cpu_count`,
  `cpu_percent`, `cpus`

#### Compose file version 2.1

- Added support for build labels. This feature is also available in the
  2.2 and 3.3 formats.

#### All formats

- Added shorthand `-u` for `--user` flag in `docker-compose exec`

- Differences in labels between the Compose file and remote network
  will now print a warning instead of preventing redeployment.

### Bugfixes

- Fixed a bug where service's dependencies were being rescaled to their
  default scale when running a `docker-compose run` command

- Fixed a bug where `docker-compose rm` with the `--stop` flag was not
  behaving properly when provided with a list of services to remove

- Fixed a bug where `cache_from` in the build section would be ignored when
  using more than one Compose file.

- Fixed a bug that prevented binding the same port to different IPs when
  using more than one Compose file.

- Fixed a bug where override files would not be picked up by Compose if they
  had the `.yaml` extension

- Fixed a bug on Windows Engine where networks would be incorrectly flagged
  for recreation

- Fixed a bug where services declaring ports would cause crashes on some
  versions of Python 3

- Fixed a bug where the output of `docker-compose config` would sometimes
  contain invalid port definitions

1.13.0 (2017-05-02)
-------------------

### Breaking changes

- `docker-compose up` now resets a service's scaling to its default value.
  You can use the newly introduced `--scale` option to specify a custom
  scale value

### New features

#### Compose file version 2.2

- Introduced version 2.2 of the `docker-compose.yml` specification. This
  version requires to be used with Docker Engine 1.13.0 or above

- Added support for `init` in service definitions.

- Added support for `scale` in service definitions. The configuration's value
  can be overridden using the `--scale` flag in `docker-compose up`.
  Please note that the `scale` command is disabled for this file format

#### Compose file version 2.x

- Added support for `options` in the `ipam` section of network definitions

### Bugfixes

- Fixed a bug where paths provided to compose via the `-f` option were not
  being resolved properly

- Fixed a bug where the `ext_ip::target_port` notation in the ports section
  was incorrectly marked as invalid

- Fixed an issue where the `exec` command would sometimes not return control
  to the terminal when using the `-d` flag

- Fixed a bug where secrets were missing from the output of the `config`
  command for v3.2 files

- Fixed an issue where `docker-compose` would hang if no internet connection
  was available

- Fixed an issue where paths containing unicode characters passed via the `-f`
  flag were causing Compose to crash

- Fixed an issue where the output of `docker-compose config` would be invalid
  if the Compose file contained external secrets

- Fixed a bug where using `--exit-code-from` with `up` would fail if Compose
  was installed in a Python 3 environment

- Fixed a bug where recreating containers using a combination of `tmpfs` and
  `volumes` would result in an invalid config state


1.12.0 (2017-04-04)
-------------------

### New features

#### Compose file version 3.2

- Introduced version 3.2 of the `docker-compose.yml` specification

- Added support for `cache_from` in the `build` section of services

- Added support for the new expanded ports syntax in service definitions

- Added support for the new expanded volumes syntax in service definitions

#### Compose file version 2.1

- Added support for `pids_limit` in service definitions

#### Compose file version 2.0 and up

- Added `--volumes` option to `docker-compose config` that lists named
  volumes declared for that project

- Added support for `mem_reservation` in service definitions (2.x only)

- Added support for `dns_opt` in service definitions (2.x only)

#### All formats

- Added a new `docker-compose images` command that lists images used by
  the current project's containers

- Added a `--stop` (shorthand `-s`) option to `docker-compose rm` that stops
  the running containers before removing them

- Added a `--resolve-image-digests` option to `docker-compose config` that
  pins the image version for each service to a permanent digest

- Added a `--exit-code-from SERVICE` option to `docker-compose up`. When
  used, `docker-compose` will exit on any container's exit with the code
  corresponding to the specified service's exit code

- Added a `--parallel` option to `docker-compose pull` that enables images
  for multiple services to be pulled simultaneously

- Added a `--build-arg` option to `docker-compose build`

- Added a `--volume <volume_mapping>` (shorthand `-v`) option to
  `docker-compose run` to declare runtime volumes to be mounted

- Added a `--project-directory PATH` option to `docker-compose` that will
  affect path resolution for the project

- When using `--abort-on-container-exit` in `docker-compose up`, the exit
  code for the container that caused the abort will be the exit code of
  the `docker-compose up` command

- Users can now configure which path separator character they want to use
  to separate the `COMPOSE_FILE` environment value using the
  `COMPOSE_PATH_SEPARATOR` environment variable

- Added support for port range to single port in port mappings
  (e.g. `8000-8010:80`)

### Bugfixes

- `docker-compose run --rm` now removes anonymous volumes after execution,
  matching the behavior of `docker run --rm`.

- Fixed a bug where override files containing port lists would cause a
  TypeError to be raised

- Fixed a bug where the `deploy` key would be missing from the output of
  `docker-compose config`

- Fixed a bug where scaling services up or down would sometimes re-use
  obsolete containers

- Fixed a bug where the output of `docker-compose config` would be invalid
  if the project declared anonymous volumes

- Variable interpolation now properly occurs in the `secrets` section of
  the Compose file

- The `secrets` section now properly appears in the output of
  `docker-compose config`

- Fixed a bug where changes to some networks properties would not be
  detected against previously created networks

- Fixed a bug where `docker-compose` would crash when trying to write into
  a closed pipe

- Fixed an issue where Compose would not pick up on the value of
  COMPOSE_TLS_VERSION when used in combination with command-line TLS flags

1.11.2 (2017-02-17)
-------------------

### Bugfixes

- Fixed a bug that was preventing secrets configuration from being
  loaded properly

- Fixed a bug where the `docker-compose config` command would fail
  if the config file contained secrets definitions

- Fixed an issue where Compose on some linux distributions would
  pick up and load an outdated version of the requests library

- Fixed an issue where socket-type files inside a build folder
  would cause `docker-compose` to crash when trying to build that
  service

- Fixed an issue where recursive wildcard patterns `**` were not being
  recognized in `.dockerignore` files.

1.11.1 (2017-02-09)
-------------------

### Bugfixes

- Fixed a bug where the 3.1 file format was not being recognized as valid
  by the Compose parser

1.11.0 (2017-02-08)
-------------------

### New Features

#### Compose file version 3.1

- Introduced version 3.1 of the `docker-compose.yml` specification. This
  version requires Docker Engine 1.13.0 or above. It introduces support
  for secrets. See the documentation for more information

#### Compose file version 2.0 and up

- Introduced the `docker-compose top` command that displays processes running
  for the different services managed by Compose.

### Bugfixes

- Fixed a bug where extending a service defining a healthcheck dictionary
  would cause `docker-compose` to error out.

- Fixed an issue where the `pid` entry in a service definition was being
  ignored when using multiple Compose files.

1.10.1 (2017-02-01)
------------------

### Bugfixes

- Fixed an issue where presence of older versions of the docker-py
  package would cause unexpected crashes while running Compose

- Fixed an issue where healthcheck dependencies would be lost when
  using multiple compose files for a project

- Fixed a few issues that made the output of the `config` command
  invalid

- Fixed an issue where adding volume labels to v3 Compose files would
  result in an error

- Fixed an issue on Windows where build context paths containing unicode
  characters were being improperly encoded

- Fixed a bug where Compose would occasionally crash while streaming logs
  when containers would stop or restart

1.10.0 (2017-01-18)
-------------------

### New Features

#### Compose file version 3.0

- Introduced version 3.0 of the `docker-compose.yml` specification. This
  version requires to be used with Docker Engine 1.13 or above and is
  specifically designed to work with the `docker stack` commands.

#### Compose file version 2.1 and up

- Healthcheck configuration can now be done in the service definition using
  the `healthcheck` parameter

- Containers dependencies can now be set up to wait on positive healthchecks
  when declared using `depends_on`. See the documentation for the updated
  syntax.
  **Note:** This feature will not be ported to version 3 Compose files.

- Added support for the `sysctls` parameter in service definitions

- Added support for the `userns_mode` parameter in service definitions

- Compose now adds identifying labels to networks and volumes it creates

#### Compose file version 2.0 and up

- Added support for the `stop_grace_period` option in service definitions.

### Bugfixes

- Colored output now works properly on Windows.

- Fixed a bug where docker-compose run would fail to set up link aliases
  in interactive mode on Windows.

- Networks created by Compose are now always made attachable
  (Compose files v2.1 and up).

- Fixed a bug where falsy values of `COMPOSE_CONVERT_WINDOWS_PATHS`
  (`0`, `false`, empty value) were being interpreted as true.

- Fixed a bug where forward slashes in some .dockerignore patterns weren't
  being parsed correctly on Windows


1.9.0 (2016-11-16)
-----------------

**Breaking changes**

- When using Compose with Docker Toolbox/Machine on Windows, volume paths are
  no longer converted from `C:\Users` to `/c/Users`-style by default. To
  re-enable this conversion so that your volumes keep working, set the
  environment variable `COMPOSE_CONVERT_WINDOWS_PATHS=1`. Users of
  Docker for Windows are not affected and do not need to set the variable.

New Features

- Interactive mode for `docker-compose run` and `docker-compose exec` is
  now supported on Windows platforms. Please note that the `docker` binary
  is required to be present on the system for this feature to work.

- Introduced version 2.1 of the `docker-compose.yml` specification. This
  version requires to be used with Docker Engine 1.12 or above.
    - Added support for setting volume labels and network labels in
  `docker-compose.yml`.
    - Added support for the `isolation` parameter in service definitions.
    - Added support for link-local IPs in the service networks definitions.
    - Added support for shell-style inline defaults in variable interpolation.
      The supported forms are `${FOO-default}` (fall back if FOO is unset) and
      `${FOO:-default}` (fall back if FOO is unset or empty).

- Added support for the `group_add` and `oom_score_adj` parameters in
  service definitions.

- Added support for the `internal` and `enable_ipv6` parameters in network
  definitions.

- Compose now defaults to using the `npipe` protocol on Windows.

- Overriding a `logging` configuration will now properly merge the `options`
  mappings if the `driver` values do not conflict.

Bug Fixes

- Fixed several bugs related to `npipe` protocol support on Windows.

- Fixed an issue with Windows paths being incorrectly converted when
  using Docker on Windows Server.

- Fixed a bug where an empty `restart` value would sometimes result in an
  exception being raised.

- Fixed an issue where service logs containing unicode characters would
  sometimes cause an error to occur.

- Fixed a bug where unicode values in environment variables would sometimes
  raise a unicode exception when retrieved.

- Fixed an issue where Compose would incorrectly detect a configuration
  mismatch for overlay networks.


1.8.1 (2016-09-22)
-----------------

Bug Fixes

- Fixed a bug where users using a credentials store were not able
  to access their private images.

- Fixed a bug where users using identity tokens to authenticate
  were not able to access their private images.

- Fixed a bug where an `HttpHeaders` entry in the docker configuration
  file would cause Compose to crash when trying to build an image.

- Fixed a few bugs related to the handling of Windows paths in volume
  binding declarations.

- Fixed a bug where Compose would sometimes crash while trying to
  read a streaming response from the engine.

- Fixed an issue where Compose would crash when encountering an API error
  while streaming container logs.

- Fixed an issue where Compose would erroneously try to output logs from
  drivers not handled by the Engine's API.

- Fixed a bug where options from the `docker-machine config` command would
  not be properly interpreted by Compose.

- Fixed a bug where the connection to the Docker Engine would
  sometimes fail when running a large number of services simultaneously.

- Fixed an issue where Compose would sometimes print a misleading
  suggestion message when running the `bundle` command.

- Fixed a bug where connection errors would not be handled properly by
  Compose during the project initialization phase.

- Fixed a bug where a misleading error would appear when encountering
  a connection timeout.


1.8.0 (2016-06-14)
-----------------

**Breaking Changes**

- As announced in 1.7.0, `docker-compose rm` now removes containers
  created by `docker-compose run` by default.

- Setting `entrypoint` on a service now empties out any default
  command that was set on the image (i.e. any `CMD` instruction in the
  Dockerfile used to build it). This makes it consistent with
  the `--entrypoint` flag to `docker run`.

New Features

- Added `docker-compose bundle`, a command that builds a bundle file
  to be consumed by the new *Docker Stack* commands in Docker 1.12.

- Added `docker-compose push`, a command that pushes service images
  to a registry.

- Compose now supports specifying a custom TLS version for
  interaction with the Docker Engine using the `COMPOSE_TLS_VERSION`
  environment variable.

Bug Fixes

- Fixed a bug where Compose would erroneously try to read `.env`
  at the project's root when it is a directory.

- `docker-compose run -e VAR` now passes `VAR` through from the shell
  to the container, as with `docker run -e VAR`.

- Improved config merging when multiple compose files are involved
  for several service sub-keys.

- Fixed a bug where volume mappings containing Windows drives would
  sometimes be parsed incorrectly.

- Fixed a bug in Windows environment where volume mappings of the
  host's root directory would be parsed incorrectly.

- Fixed a bug where `docker-compose config` would output an invalid
  Compose file if external networks were specified.

- Fixed an issue where unset buildargs would be assigned a string
  containing `'None'` instead of the expected empty value.

- Fixed a bug where yes/no prompts on Windows would not show before
  receiving input.

- Fixed a bug where trying to `docker-compose exec` on Windows
  without the `-d` option would exit with a stacktrace. This will
  still fail for the time being, but should do so gracefully.

- Fixed a bug where errors during `docker-compose up` would show
  an unrelated stacktrace at the end of the process.

- `docker-compose create` and `docker-compose start` show more
  descriptive error messages when something goes wrong.


1.7.1 (2016-05-04)
-----------------

Bug Fixes

- Fixed a bug where the output of `docker-compose config` for v1 files
  would be an invalid configuration file.

- Fixed a bug where `docker-compose config` would not check the validity
  of links.

- Fixed an issue where `docker-compose help` would not output a list of
  available commands and generic options as expected.

- Fixed an issue where filtering by service when using `docker-compose logs`
  would not apply for newly created services.

- Fixed a bug where unchanged services would sometimes be recreated in
  in the up phase when using Compose with Python 3.

- Fixed an issue where API errors encountered during the up phase would
  not be recognized as a failure state by Compose.

- Fixed a bug where Compose would raise a NameError because of an undefined
  exception name on non-Windows platforms.

- Fixed a bug where the wrong version of `docker-py` would sometimes be
  installed alongside Compose.

- Fixed a bug where the host value output by `docker-machine config default`
  would not be recognized as valid options by the `docker-compose`
  command line.

- Fixed an issue where Compose would sometimes exit unexpectedly  while
  reading events broadcasted by a Swarm cluster.

- Corrected a statement in the docs about the location of the `.env` file,
  which is indeed read from the current directory, instead of in the same
  location as the Compose file.


1.7.0 (2016-04-13)
------------------

**Breaking Changes**

-   `docker-compose logs` no longer follows log output by default. It now
    matches the behaviour of `docker logs` and exits after the current logs
    are printed. Use `-f` to get the old default behaviour.

-   Booleans are no longer allows as values for mappings in the Compose file
    (for keys `environment`, `labels` and `extra_hosts`). Previously this
    was a warning. Boolean values should be quoted so they become string values.

New Features

-   Compose now looks for a `.env` file in the directory where it's run and
    reads any environment variables defined inside, if they're not already
    set in the shell environment. This lets you easily set defaults for
    variables used in the Compose file, or for any of the `COMPOSE_*` or
    `DOCKER_*` variables.

-   Added a `--remove-orphans` flag to both `docker-compose up` and
    `docker-compose down` to remove containers for services that were removed
    from the Compose file.

-   Added a `--all` flag to `docker-compose rm` to include containers created
    by `docker-compose run`. This will become the default behavior in the next
    version of Compose.

-   Added support for all the same TLS configuration flags used by the `docker`
    client: `--tls`, `--tlscert`, `--tlskey`, etc.

-   Compose files now support the `tmpfs` and `shm_size` options.

-   Added the `--workdir` flag to `docker-compose run`

-   `docker-compose logs` now shows logs for new containers that are created
    after it starts.

-   The `COMPOSE_FILE` environment variable can now contain multiple files,
    separated by the host system's standard path separator (`:` on Mac/Linux,
    `;` on Windows).

-   You can now specify a static IP address when connecting a service to a
    network with the `ipv4_address` and `ipv6_address` options.

-   Added `--follow`, `--timestamp`, and `--tail` flags to the
    `docker-compose logs` command.

-   `docker-compose up`, and `docker-compose start` will now start containers
    in parallel where possible.

-   `docker-compose stop` now stops containers in reverse dependency order
    instead of all at once.

-   Added the `--build` flag to `docker-compose up` to force it to build a new
    image. It now shows a warning if an image is automatically built when the
    flag is not used.

-   Added the `docker-compose exec` command for executing a process in a running
    container.


Bug Fixes

-   `docker-compose down` now removes containers created by
    `docker-compose run`.

-   A more appropriate error is shown when a timeout is hit during `up` when
    using a tty.

-   Fixed a bug in `docker-compose down` where it would abort if some resources
    had already been removed.

-   Fixed a bug where changes to network aliases would not trigger a service
    to be recreated.

-   Fix a bug where a log message was printed about creating a new volume
    when it already existed.

-   Fixed a bug where interrupting `up` would not always shut down containers.

-   Fixed a bug where `log_opt` and `log_driver` were not properly carried over
    when extending services in the v1 Compose file format.

-   Fixed a bug where empty values for build args would cause file validation
    to fail.

1.6.2 (2016-02-23)
------------------

-   Fixed a bug where connecting to a TLS-enabled Docker Engine would fail with
    a certificate verification error.

1.6.1 (2016-02-23)
------------------

Bug Fixes

-   Fixed a bug where recreating a container multiple times would cause the
    new container to be started without the previous volumes.

-   Fixed a bug where Compose would set the value of unset environment variables
    to an empty string, instead of a key without a value.

-   Provide a better error message when Compose requires a more recent version
    of the Docker API.

-   Add a missing config field `network.aliases` which allows setting a network
    scoped alias for a service.

-   Fixed a bug where `run` would not start services listed in `depends_on`.

-   Fixed a bug where `networks` and `network_mode` where not merged when using
    extends or multiple Compose files.

-   Fixed a bug with service aliases where the short container id alias was
    only contained 10 characters, instead of the 12 characters used in previous
    versions.

-   Added a missing log message when creating a new named volume.

-   Fixed a bug where `build.args` was not merged when using `extends` or
    multiple Compose files.

-   Fixed some bugs with config validation when null values or incorrect types
    were used instead of a mapping.

-   Fixed a bug where a `build` section without a `context` would show a stack
    trace instead of a helpful validation message.

-   Improved compatibility with swarm by only setting a container affinity to
    the previous instance of a services' container when the service uses an
    anonymous container volume. Previously the affinity was always set on all
    containers.

-   Fixed the validation of some `driver_opts` would cause an error if a number
    was used instead of a string.

-   Some improvements to the `run.sh` script used by the Compose container install
    option.

-   Fixed a bug with `up --abort-on-container-exit` where Compose would exit,
    but would not stop other containers.

-   Corrected the warning message that is printed when a boolean value is used
    as a value in a mapping.


1.6.0 (2016-01-15)
------------------

Major Features:

-   Compose 1.6 introduces a new format for `docker-compose.yml` which lets
    you define networks and volumes in the Compose file as well as services. It
    also makes a few changes to the structure of some configuration options.

    You don't have to use it - your existing Compose files will run on Compose
    1.6 exactly as they do today.

    Check the upgrade guide for full details:
    https://docs.docker.com/compose/compose-file#upgrading

-   Support for networking has exited experimental status and is the recommended
    way to enable communication between containers.

    If you use the new file format, your app will use networking. If you aren't
    ready yet, just leave your Compose file as it is and it'll continue to work
    just the same.

    By default, you don't have to configure any networks. In fact, using
    networking with Compose involves even less configuration than using links.
    Consult the networking guide for how to use it:
    https://docs.docker.com/compose/networking

    The experimental flags `--x-networking` and `--x-network-driver`, introduced
    in Compose 1.5, have been removed.

-   You can now pass arguments to a build if you're using the new file format:

        build:
          context: .
          args:
            buildno: 1

-   You can now specify both a `build` and an `image` key if you're using the
    new file format. `docker-compose build` will build the image and tag it with
    the name you've specified, while `docker-compose pull` will attempt to pull
    it.

-   There's a new `events` command for monitoring container events from
    the application, much like `docker events`. This is a good primitive for
    building tools on top of Compose for performing actions when particular
    things happen, such as containers starting and stopping.

-   There's a new `depends_on` option for specifying dependencies between
    services. This enforces the order of startup, and ensures that when you run
    `docker-compose up SERVICE` on a service with dependencies, those are started
    as well.

New Features:

-   Added a new command `config` which validates and prints the Compose
    configuration after interpolating variables, resolving relative paths, and
    merging multiple files and `extends`.

-   Added a new command `create` for creating containers without starting them.

-   Added a new command `down` to stop and remove all the resources created by
    `up` in a single command.

-   Added support for the `cpu_quota` configuration option.

-   Added support for the `stop_signal` configuration option.

-   Commands `start`, `restart`, `pause`, and `unpause` now exit with an
    error status code if no containers were modified.

-   Added a new `--abort-on-container-exit` flag to `up` which causes `up` to
    stop all container and exit once the first container exits.

-   Removed support for `FIG_FILE`, `FIG_PROJECT_NAME`, and no longer reads
    `fig.yml` as a default Compose file location.

-   Removed the `migrate-to-labels` command.

-   Removed the `--allow-insecure-ssl` flag.


Bug Fixes:

-   Fixed a validation bug that prevented the use of a range of ports in
    the `expose` field.

-   Fixed a validation bug that prevented the use of arrays in the `entrypoint`
    field if they contained duplicate entries.

-   Fixed a bug that caused `ulimits` to be ignored when used with `extends`.

-   Fixed a bug that prevented ipv6 addresses in `extra_hosts`.

-   Fixed a bug that caused `extends` to be ignored when included from
    multiple Compose files.

-   Fixed an incorrect warning when a container volume was defined in
    the Compose file.

-   Fixed a bug that prevented the force shutdown behaviour of `up` and
    `logs`.

-   Fixed a bug that caused `None` to be printed as the network driver name
    when the default network driver was used.

-   Fixed a bug where using the string form of `dns` or `dns_search` would
    cause an error.

-   Fixed a bug where a container would be reported as "Up" when it was
    in the restarting state.

-   Fixed a confusing error message when DOCKER_CERT_PATH was not set properly.

-   Fixed a bug where attaching to a container would fail if it was using a
    non-standard logging driver (or none at all).


1.5.2 (2015-12-03)
------------------

-   Fixed a bug which broke the use of `environment` and `env_file` with
    `extends`, and caused environment keys without values to have a `None`
    value, instead of a value from the host environment.

-   Fixed a regression in 1.5.1 that caused a warning about volumes to be
    raised incorrectly when containers were recreated.

-   Fixed a bug which prevented building a `Dockerfile` that used `ADD <url>`

-   Fixed a bug with `docker-compose restart` which prevented it from
    starting stopped containers.

-   Fixed handling of SIGTERM and SIGINT to properly stop containers

-   Add support for using a url as the value of `build`

-   Improved the validation of the `expose` option


1.5.1 (2015-11-12)
------------------

-   Add the `--force-rm` option to `build`.

-   Add the `ulimit` option for services in the Compose file.

-   Fixed a bug where `up` would error with "service needs to be built" if
    a service changed from using `image` to using `build`.

-   Fixed a bug that would cause incorrect output of parallel operations
    on some terminals.

-   Fixed a bug that prevented a container from being recreated when the
    mode of a `volumes_from` was changed.

-   Fixed a regression in 1.5.0 where non-utf-8 unicode characters would cause
    `up` or `logs` to crash.

-   Fixed a regression in 1.5.0 where Compose would use a success exit status
    code when a command fails due to an HTTP timeout communicating with the
    docker daemon.

-   Fixed a regression in 1.5.0 where `name` was being accepted as a valid
    service option which would override the actual name of the service.

-   When using `--x-networking` Compose no longer sets the hostname to the
    container name.

-   When using `--x-networking` Compose will only create the default network
    if at least one container is using the network.

-   When printings logs during `up` or `logs`, flush the output buffer after
    each line to prevent buffering issues from hiding logs.

-   Recreate a container if one of its dependencies is being created.
    Previously a container was only recreated if it's dependencies already
    existed, but were being recreated as well.

-   Add a warning when a `volume` in the Compose file is being ignored
    and masked by a container volume from a previous container.

-   Improve the output of `pull` when run without a tty.

-   When using multiple Compose files, validate each before attempting to merge
    them together. Previously invalid files would result in not helpful errors.

-   Allow dashes in keys in the `environment` service option.

-   Improve validation error messages by including the filename as part of the
    error message.


1.5.0 (2015-11-03)
------------------

**Breaking changes:**

With the introduction of variable substitution support in the Compose file, any
Compose file that uses an environment variable (`$VAR` or `${VAR}`) in the `command:`
or `entrypoint:` field will break.

Previously these values were interpolated inside the container, with a value
from the container environment.  In Compose 1.5.0, the values will be
interpolated on the host, with a value from the host environment.

To migrate a Compose file to 1.5.0, escape the variables with an extra `$`
(ex: `$$VAR` or `$${VAR}`).  See
https://github.com/docker/compose/blob/8cc8e61/docs/compose-file.md#variable-substitution

Major features:

-   Compose is now available for Windows.

-   Environment variables can be used in the Compose file. See
    https://github.com/docker/compose/blob/8cc8e61/docs/compose-file.md#variable-substitution

-   Multiple compose files can be specified, allowing you to override
    settings in the default Compose file. See
    https://github.com/docker/compose/blob/8cc8e61/docs/reference/docker-compose.md
    for more details.

-   Compose now produces better error messages when a file contains
    invalid configuration.

-   `up` now waits for all services to exit before shutting down,
    rather than shutting down as soon as one container exits.

-   Experimental support for the new docker networking system can be
    enabled with the `--x-networking` flag. Read more here:
    https://github.com/docker/docker/blob/8fee1c20/docs/userguide/dockernetworks.md

New features:

-   You can now optionally pass a mode to `volumes_from`, e.g.
    `volumes_from: ["servicename:ro"]`.

-   Since Docker now lets you create volumes with names, you can refer to those
    volumes by name in `docker-compose.yml`. For example,
    `volumes: ["mydatavolume:/data"]` will mount the volume named
    `mydatavolume` at the path `/data` inside the container.

    If the first component of an entry in `volumes` starts with a `.`, `/` or
    `~`, it is treated as a path and expansion of relative paths is performed as
    necessary. Otherwise, it is treated as a volume name and passed straight
    through to Docker.

    Read more on named volumes and volume drivers here:
    https://github.com/docker/docker/blob/244d9c33/docs/userguide/dockervolumes.md

-   `docker-compose build --pull` instructs Compose to pull the base image for
    each Dockerfile before building.

-   `docker-compose pull --ignore-pull-failures` instructs Compose to continue
    if it fails to pull a single service's image, rather than aborting.

-   You can now specify an IPC namespace in `docker-compose.yml` with the `ipc`
    option.

-   Containers created by `docker-compose run` can now be named with the
    `--name` flag.

-   If you install Compose with pip or use it as a library, it now works with
    Python 3.

-   `image` now supports image digests (in addition to ids and tags), e.g.
    `image: "busybox@sha256:38a203e1986cf79639cfb9b2e1d6e773de84002feea2d4eb006b52004ee8502d"`

-   `ports` now supports ranges of ports, e.g.

        ports:
          - "3000-3005"
          - "9000-9001:8000-8001"

-   `docker-compose run` now supports a `-p|--publish` parameter, much like
    `docker run -p`, for publishing specific ports to the host.

-   `docker-compose pause` and `docker-compose unpause` have been implemented,
    analogous to `docker pause` and `docker unpause`.

-   When using `extends` to copy configuration from another service in the same
    Compose file, you can omit the `file` option.

-   Compose can be installed and run as a Docker image. This is an experimental
    feature.

Bug fixes:

-   All values for the `log_driver` option which are supported by the Docker
    daemon are now supported by Compose.

-   `docker-compose build` can now be run successfully against a Swarm cluster.


1.4.2 (2015-09-22)
------------------

-  Fixed a regression in the 1.4.1 release that would cause `docker-compose up`
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
- `security_opt`, like `docker run --security-opt`, lets you specify [security options](https://docs.docker.com/engine/reference/run/#security-configuration).
- `log_driver`, like `docker run --log-driver`, lets you specify a [log driver](https://docs.docker.com/engine/reference/run/#logging-drivers-log-driver).

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
- If you’re installing via PyPI, the package is now `docker-compose`, so install it with `pip install docker-compose`.

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
