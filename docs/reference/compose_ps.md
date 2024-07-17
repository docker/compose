# docker compose ps

<!---MARKER_GEN_START-->
Lists containers for a Compose project, with current status and exposed ports.

```console
$ docker compose ps
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-foo-1   alpine    "/entrypoint.…"   foo        4 seconds ago   Up 2 seconds    0.0.0.0:8080->80/tcp
```

By default, only running containers are shown. `--all` flag can be used to include stopped containers.

```console
$ docker compose ps --all
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-foo-1   alpine    "/entrypoint.…"   foo        4 seconds ago   Up 2 seconds    0.0.0.0:8080->80/tcp
example-bar-1   alpine    "/entrypoint.…"   bar        4 seconds ago   exited (0)
```

### Options

| Name                  | Type          | Default | Description                                                                                                                                                                                                                                                                                                                                                                                                                          |
|:----------------------|:--------------|:--------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `-a`, `--all`         | `bool`        |         | Show all stopped containers (including those created by the run command)                                                                                                                                                                                                                                                                                                                                                             |
| `--dry-run`           | `bool`        |         | Execute command in dry run mode                                                                                                                                                                                                                                                                                                                                                                                                      |
| [`--filter`](#filter) | `string`      |         | Filter services by a property (supported filters: status)                                                                                                                                                                                                                                                                                                                                                                            |
| [`--format`](#format) | `string`      | `table` | Format output using a custom template:<br>'table':            Print output in table format with column headers (default)<br>'table TEMPLATE':   Print output in table format using the given Go template<br>'json':             Print in JSON format<br>'TEMPLATE':         Print output using the given Go template.<br>Refer to https://docs.docker.com/go/formatting/ for more information about formatting output with templates |
| `--no-trunc`          | `bool`        |         | Don't truncate output                                                                                                                                                                                                                                                                                                                                                                                                                |
| `--orphans`           | `bool`        | `true`  | Include orphaned services (not declared by project)                                                                                                                                                                                                                                                                                                                                                                                  |
| `-q`, `--quiet`       | `bool`        |         | Only display IDs                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `--services`          | `bool`        |         | Display services                                                                                                                                                                                                                                                                                                                                                                                                                     |
| [`--status`](#status) | `stringArray` |         | Filter services by status. Values: [paused \| restarting \| removing \| running \| dead \| created \| exited]                                                                                                                                                                                                                                                                                                                        |


<!---MARKER_GEN_END-->

## Description

Lists containers for a Compose project, with current status and exposed ports.

```console
$ docker compose ps
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-foo-1   alpine    "/entrypoint.…"   foo        4 seconds ago   Up 2 seconds    0.0.0.0:8080->80/tcp
```

By default, only running containers are shown. `--all` flag can be used to include stopped containers.

```console
$ docker compose ps --all
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-foo-1   alpine    "/entrypoint.…"   foo        4 seconds ago   Up 2 seconds    0.0.0.0:8080->80/tcp
example-bar-1   alpine    "/entrypoint.…"   bar        4 seconds ago   exited (0)
```

## Examples

### <a name="format"></a> Format the output (--format)

By default, the `docker compose ps` command uses a table ("pretty") format to
show the containers. The `--format` flag allows you to specify alternative
presentations for the output. Currently, supported options are `pretty` (default),
and `json`, which outputs information about the containers as a JSON array:

```console
$ docker compose ps --format json
[{"ID":"1553b0236cf4d2715845f053a4ee97042c4f9a2ef655731ee34f1f7940eaa41a","Name":"example-bar-1","Command":"/docker-entrypoint.sh nginx -g 'daemon off;'","Project":"example","Service":"bar","State":"exited","Health":"","ExitCode":0,"Publishers":null},{"ID":"f02a4efaabb67416e1ff127d51c4b5578634a0ad5743bd65225ff7d1909a3fa0","Name":"example-foo-1","Command":"/docker-entrypoint.sh nginx -g 'daemon off;'","Project":"example","Service":"foo","State":"running","Health":"","ExitCode":0,"Publishers":[{"URL":"0.0.0.0","TargetPort":80,"PublishedPort":8080,"Protocol":"tcp"}]}]
```

The JSON output allows you to use the information in other tools for further
processing, for example, using the [`jq` utility](https://stedolan.github.io/jq/)
to pretty-print the JSON:

```console
$ docker compose ps --format json | jq .
[
  {
    "ID": "1553b0236cf4d2715845f053a4ee97042c4f9a2ef655731ee34f1f7940eaa41a",
    "Name": "example-bar-1",
    "Command": "/docker-entrypoint.sh nginx -g 'daemon off;'",
    "Project": "example",
    "Service": "bar",
    "State": "exited",
    "Health": "",
    "ExitCode": 0,
    "Publishers": null
  },
  {
    "ID": "f02a4efaabb67416e1ff127d51c4b5578634a0ad5743bd65225ff7d1909a3fa0",
    "Name": "example-foo-1",
    "Command": "/docker-entrypoint.sh nginx -g 'daemon off;'",
    "Project": "example",
    "Service": "foo",
    "State": "running",
    "Health": "",
    "ExitCode": 0,
    "Publishers": [
      {
        "URL": "0.0.0.0",
        "TargetPort": 80,
        "PublishedPort": 8080,
        "Protocol": "tcp"
      }
    ]
  }
]
```

### <a name="status"></a> Filter containers by status (--status)

Use the `--status` flag to filter the list of containers by status. For example,
to show only containers that are running or only containers that have exited:

```console
$ docker compose ps --status=running
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-foo-1   alpine    "/entrypoint.…"   foo        4 seconds ago   Up 2 seconds    0.0.0.0:8080->80/tcp

$ docker compose ps --status=exited
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-bar-1   alpine    "/entrypoint.…"   bar        4 seconds ago   exited (0)
```

### <a name="filter"></a> Filter containers by status (--filter)

The [`--status` flag](#status) is a convenient shorthand for the `--filter status=<status>`
flag. The example below is the equivalent to the example from the previous section,
this time using the `--filter` flag:

```console
$ docker compose ps --filter status=running
NAME            IMAGE     COMMAND           SERVICE    CREATED         STATUS          PORTS
example-foo-1   alpine    "/entrypoint.…"   foo        4 seconds ago   Up 2 seconds    0.0.0.0:8080->80/tcp
```

The `docker compose ps` command currently only supports the `--filter status=<status>`
option, but additional filter options may be added in the future.
