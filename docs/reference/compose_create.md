# docker compose create

<!---MARKER_GEN_START-->
Creates containers for a service

### Options

| Name               | Type          | Default  | Description                                                                                   |
|:-------------------|:--------------|:---------|:----------------------------------------------------------------------------------------------|
| `--build`          |               |          | Build images before starting containers                                                       |
| `--dry-run`        |               |          | Execute command in dry run mode                                                               |
| `--force-recreate` |               |          | Recreate containers even if their configuration and image haven't changed                     |
| `--no-build`       |               |          | Don't build an image, even if it's policy                                                     |
| `--no-recreate`    |               |          | If containers already exist, don't recreate them. Incompatible with --force-recreate.         |
| `--pull`           | `string`      | `policy` | Pull image before running ("always"\|"missing"\|"never"\|"build")                             |
| `--quiet-pull`     |               |          | Pull without printing progress information                                                    |
| `--remove-orphans` |               |          | Remove containers for services not defined in the Compose file                                |
| `--scale`          | `stringArray` |          | Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present. |


<!---MARKER_GEN_END-->

