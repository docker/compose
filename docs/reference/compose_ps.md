# docker compose ps

<!---MARKER_GEN_START-->
List containers

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `-a`, `--all` |  |  | Show all stopped containers (including those created by the run command) |
| `--format` | `string` | `pretty` | Format the output. Values: [pretty \| json] |
| `-q`, `--quiet` |  |  | Only display IDs |
| `--services` |  |  | Display services |
| `--status` | `stringArray` |  | Filter services by status. Values: [paused \| restarting \| removing \| running \| dead \| created \| exited] |


<!---MARKER_GEN_END-->

## Description

Lists containers for a Compose project, with current status and exposed ports.

```console
$ docker compose ps
NAME                SERVICE             STATUS              PORTS
example_foo_1       foo                 running (healthy)   0.0.0.0:8000->80/tcp
example_bar_1       bar                 exited (1)
```
