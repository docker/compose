# docker compose build

<!---MARKER_GEN_START-->
Build or rebuild services

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `--build-arg` | `stringArray` |  | Set build-time variables for services. |
| `--no-cache` |  |  | Do not use cache when building the image |
| `--progress` | `string` | `auto` | Set type of progress output (auto, tty, plain, quiet) |
| `--pull` |  |  | Always attempt to pull a newer version of the image. |
| `-q`, `--quiet` |  |  | Don't print anything to STDOUT |
| `--ssh` | `string` |  | Set SSH authentications used when building service images. (use 'default' for using your default SSH Agent) |


<!---MARKER_GEN_END-->

## Description

Services are built once and then tagged, by default as `project_service`.

If the Compose file specifies an
[image](https://github.com/compose-spec/compose-spec/blob/master/spec.md#image) name,
the image is tagged with that name, substituting any variables beforehand. See
[variable interpolation](https://github.com/compose-spec/compose-spec/blob/master/spec.md#interpolation).

If you change a service's `Dockerfile` or the contents of its build directory,
run `docker compose build` to rebuild it.

### Native build using the docker CLI

Compose by default uses the `docker` CLI to perform builds (also known as "native
build"). By using the `docker` CLI, Compose can take advantage of features such
as [BuildKit](../../develop/develop-images/build_enhancements.md), which are not
supported by Compose itself. BuildKit is enabled by default on Docker Desktop,
but requires the `DOCKER_BUILDKIT=1` environment variable to be set on other
platforms.

Refer to the [Compose CLI environment variables](envvars.md#compose_docker_cli_build)
section to learn how to switch between "native build" and "compose build".
