# docker compose stats

<!---MARKER_GEN_START-->
Display a live stream of container(s) resource usage statistics

### Options

| Name          | Type     | Default | Description                                                                                                                                                                                                                                                                                                                                                                                                                          |
|:--------------|:---------|:--------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `-a`, `--all` | `bool`   |         | Show all containers (default shows just running)                                                                                                                                                                                                                                                                                                                                                                                     |
| `--dry-run`   | `bool`   |         | Execute command in dry run mode                                                                                                                                                                                                                                                                                                                                                                                                      |
| `--format`    | `string` |         | Format output using a custom template:<br>'table':            Print output in table format with column headers (default)<br>'table TEMPLATE':   Print output in table format using the given Go template<br>'json':             Print in JSON format<br>'TEMPLATE':         Print output using the given Go template.<br>Refer to https://docs.docker.com/go/formatting/ for more information about formatting output with templates |
| `--no-stream` | `bool`   |         | Disable streaming stats and only pull the first result                                                                                                                                                                                                                                                                                                                                                                               |
| `--no-trunc`  | `bool`   |         | Do not truncate output                                                                                                                                                                                                                                                                                                                                                                                                               |


<!---MARKER_GEN_END-->

