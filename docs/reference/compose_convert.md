# docker compose convert

<!---MARKER_GEN_START-->
Converts the compose file to platform's canonical format

### Aliases

`docker compose convert`, `docker compose config`

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `--format` | `string` | `yaml` | Format the output. Values: [yaml \| json] |
| `--hash` | `string` |  | Print the service config hash, one per line. |
| `--images` |  |  | Print the image names, one per line. |
| `--no-interpolate` |  |  | Don't interpolate environment variables. |
| `--no-normalize` |  |  | Don't normalize compose model. |
| `-o`, `--output` | `string` |  | Save to file (default to stdout) |
| `--profiles` |  |  | Print the profile names, one per line. |
| `-q`, `--quiet` |  |  | Only validate the configuration, don't print anything. |
| `--resolve-image-digests` |  |  | Pin image tags to digests. |
| `--services` |  |  | Print the service names, one per line. |
| `--volumes` |  |  | Print the volume names, one per line. |


<!---MARKER_GEN_END-->

## Description

`docker compose convert` render the actual data model to be applied on target platform. When used with Docker engine,
it merges the Compose files set by `-f` flags, resolves variables in Compose file, and expands short-notation into
fully defined Compose model.

To allow smooth migration from docker-compose, this subcommand declares alias `docker compose config`
