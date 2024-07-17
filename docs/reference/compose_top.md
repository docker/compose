# docker compose top

<!---MARKER_GEN_START-->
Displays the running processes

### Options

| Name        | Type   | Default | Description                     |
|:------------|:-------|:--------|:--------------------------------|
| `--dry-run` | `bool` |         | Execute command in dry run mode |


<!---MARKER_GEN_END-->

## Description

Displays the running processes

## Examples

```console
$ docker compose top
example_foo_1
UID    PID      PPID     C    STIME   TTY   TIME       CMD
root   142353   142331   2    15:33   ?     00:00:00   ping localhost -c 5
```
