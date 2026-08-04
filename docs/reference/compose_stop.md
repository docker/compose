# docker compose stop

<!---MARKER_GEN_START-->
Stops running containers without removing them. They can be started again with `docker compose start`.

### Options

| Name              | Type   | Default | Description                                                                                              |
|:------------------|:-------|:--------|:---------------------------------------------------------------------------------------------------------|
| `--dry-run`       | `bool` |         | Execute command in dry run mode                                                                          |
| `--profiles-only` | `bool` |         | Only stop services enabled by a profile, leaving other services running (all profiles if none is active) |
| `-t`, `--timeout` | `int`  | `0`     | Specify a shutdown timeout in seconds                                                                    |


<!---MARKER_GEN_END-->

## Description

Stops running containers without removing them. They can be started again with `docker compose start`.
