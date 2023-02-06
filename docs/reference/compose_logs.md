# docker compose logs

<!---MARKER_GEN_START-->
View output from containers

### Options

| Name                 | Type     | Default | Description                                                                                    |
|:---------------------|:---------|:--------|:-----------------------------------------------------------------------------------------------|
| `--dry-run`          |          |         | Execute command in dry run mode                                                                |
| `-f`, `--follow`     |          |         | Follow log output.                                                                             |
| `--index`            | `int`    | `0`     | index of the container if service has multiple replicas                                        |
| `--no-color`         |          |         | Produce monochrome output.                                                                     |
| `--no-log-prefix`    |          |         | Don't print prefix in logs.                                                                    |
| `--since`            | `string` |         | Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)    |
| `-n`, `--tail`       | `string` | `all`   | Number of lines to show from the end of the logs for each container.                           |
| `-t`, `--timestamps` |          |         | Show timestamps.                                                                               |
| `--until`            | `string` |         | Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes) |


<!---MARKER_GEN_END-->

## Description

Displays log output from services.