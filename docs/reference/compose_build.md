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
