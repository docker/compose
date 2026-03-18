# docker compose deploy

<!---MARKER_GEN_START-->
Deploy a Compose application to a Docker server.

This command applies the Compose project to the target Docker server,
recreating containers with updated configuration and images. Images are
pulled from the registry unless --build is specified.

Use health checks defined in the Compose file to ensure zero-downtime
deployments by passing --wait.

### Options

| Name               | Type   | Default | Description                                                                     |
|:-------------------|:-------|:--------|:--------------------------------------------------------------------------------|
| `--build`          | `bool` |         | Build images before deploying                                                   |
| `--dry-run`        | `bool` |         | Execute command in dry run mode                                                 |
| `--no-build`       | `bool` |         | Do not build images even if build configuration is defined                      |
| `--push`           | `bool` |         | Push images to registry before deploying                                        |
| `-q`, `--quiet`    | `bool` |         | Suppress pull/push progress output                                              |
| `--remove-orphans` | `bool` |         | Remove containers for services not defined in the Compose file                  |
| `--wait`           | `bool` |         | Wait for services to be healthy before returning                                |
| `--wait-timeout`   | `int`  | `0`     | Maximum duration in seconds to wait for services to be healthy (0 = no timeout) |


<!---MARKER_GEN_END-->

