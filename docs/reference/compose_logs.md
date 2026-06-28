# docker compose logs

<!---MARKER_GEN_START-->
Displays log output from services

### Options

| Name                                                                                                                                                                       | Type     | Default | Description                                                                                    |
|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:---------|:--------|:-----------------------------------------------------------------------------------------------|
| `--dry-run`                                                                                                                                                                | `bool`   |         | Execute command in dry run mode                                                                |
| [`-f`](https://docs.docker.com/reference/cli/docker/container/logs/#follow), [`--follow`](https://docs.docker.com/reference/cli/docker/container/logs/#follow)             | `bool`   |         | Follow log output                                                                              |
| `--index`                                                                                                                                                                  | `int`    | `0`     | index of the container if service has multiple replicas                                        |
| `--no-color`                                                                                                                                                               | `bool`   |         | Produce monochrome output                                                                      |
| `--no-log-prefix`                                                                                                                                                          | `bool`   |         | Don't print prefix in logs                                                                     |
| [`--since`](https://docs.docker.com/reference/cli/docker/container/logs/#since)                                                                                            | `string` |         | Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)    |
| [`-n`](https://docs.docker.com/reference/cli/docker/container/logs/#tail), [`--tail`](https://docs.docker.com/reference/cli/docker/container/logs/#tail)                   | `string` | `all`   | Number of lines to show from the end of the logs for each container                            |
| [`-t`](https://docs.docker.com/reference/cli/docker/container/logs/#timestamps), [`--timestamps`](https://docs.docker.com/reference/cli/docker/container/logs/#timestamps) | `bool`   |         | Show timestamps                                                                                |
| [`--until`](https://docs.docker.com/reference/cli/docker/container/logs/#until)                                                                                            | `string` |         | Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes) |


<!---MARKER_GEN_END-->

## Description

Displays log output from services
