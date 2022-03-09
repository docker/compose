# docker compose rm

<!---MARKER_GEN_START-->
Removes stopped service containers

By default, anonymous volumes attached to containers will not be removed. You
can override this with -v. To list all volumes, use "docker volume ls".

Any data which is not in a volume will be lost.

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `-f`, `--force` |  |  | Don't ask to confirm removal |
| `-s`, `--stop` |  |  | Stop the containers, if required, before removing |
| `-v`, `--volumes` |  |  | Remove any anonymous volumes attached to containers |


<!---MARKER_GEN_END-->

## Description

Removes stopped service containers.

By default, anonymous volumes attached to containers are not removed. You can override this with `-v`. To list all
volumes, use `docker volume ls`.

Any data which is not in a volume is lost.

Running the command with no options also removes one-off containers created by `docker compose run`:

```console
$ docker compose rm
Going to remove djangoquickstart_web_run_1
Are you sure? [yN] y
Removing djangoquickstart_web_run_1 ... done
```
