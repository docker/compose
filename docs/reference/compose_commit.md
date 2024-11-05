# docker compose commit

<!---MARKER_GEN_START-->
Create a new image from a service container's changes

### Options

| Name              | Type     | Default | Description                                                |
|:------------------|:---------|:--------|:-----------------------------------------------------------|
| `-a`, `--author`  | `string` |         | Author (e.g., "John Hannibal Smith <hannibal@a-team.com>") |
| `-c`, `--change`  | `list`   |         | Apply Dockerfile instruction to the created image          |
| `--dry-run`       | `bool`   |         | Execute command in dry run mode                            |
| `--index`         | `int`    | `0`     | index of the container if service has multiple replicas.   |
| `-m`, `--message` | `string` |         | Commit message                                             |
| `-p`, `--pause`   | `bool`   | `true`  | Pause container during commit                              |


<!---MARKER_GEN_END-->

