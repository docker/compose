# docker compose convert

<!---MARKER_GEN_START-->
Parse, resolve and render compose file in canonical format

### Aliases

`docker compose config`, `docker compose convert`

### Options

| Name                      | Type     | Default | Description                                                                 |
|:--------------------------|:---------|:--------|:----------------------------------------------------------------------------|
| `--dry-run`               |          |         | Execute command in dry run mode                                             |
| `--environment`           |          |         | Print environment used for interpolation.                                   |
| `--format`                | `string` | `yaml`  | Format the output. Values: [yaml \| json]                                   |
| `--hash`                  | `string` |         | Print the service config hash, one per line.                                |
| `--images`                |          |         | Print the image names, one per line.                                        |
| `--no-consistency`        |          |         | Don't check model consistency - warning: may produce invalid Compose output |
| `--no-interpolate`        |          |         | Don't interpolate environment variables                                     |
| `--no-normalize`          |          |         | Don't normalize compose model                                               |
| `--no-path-resolution`    |          |         | Don't resolve file paths                                                    |
| `-o`, `--output`          | `string` |         | Save to file (default to stdout)                                            |
| `--profiles`              |          |         | Print the profile names, one per line.                                      |
| `-q`, `--quiet`           |          |         | Only validate the configuration, don't print anything                       |
| `--resolve-image-digests` |          |         | Pin image tags to digests                                                   |
| `--services`              |          |         | Print the service names, one per line.                                      |
| `--variables`             |          |         | Print model variables and default values.                                   |
| `--volumes`               |          |         | Print the volume names, one per line.                                       |


<!---MARKER_GEN_END-->

## Description

`docker compose config` renders the actual data model to be applied on the Docker Engine.
It merges the Compose files set by `-f` flags, resolves variables in the Compose file, and expands short-notation into
the canonical format.
