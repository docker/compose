# `secrets`/`configs`: `driver: copy`

## Problem

`secrets.<name>.file` and `configs.<name>.file` are, by default, interpreted as a path on the **Docker host**:
Compose creates a read-only bind mount from that host path into the container. This works fine when the
daemon is local, but breaks entirely once `DOCKER_HOST` or a Docker context points at a remote engine —
Compose is a client-side tool and cannot bind-mount a path that only exists on the client's machine into a
container running on a different host.

```yaml
services:
  app:
    secrets:
      - db_password

secrets:
  db_password:
    file: ./db_password.txt   # only works if the daemon is local
```

Against a remote `DOCKER_HOST`, the above fails (or silently mounts the wrong file, if a path with the same
name happens to exist on the remote host).

## `driver: copy`

Setting `driver: copy` on a secret or config opts it into being read from the **client's** local filesystem and
copied into the container, instead of bind-mounted from the Docker host:

```yaml
secrets:
  db_password:
    file: ./db_password.txt
    driver: copy
```

This is an explicit **opt-in**, not the default, and this is deliberate: existing users may rely on the current
bind-mount behavior, e.g. to edit the file on the Docker host directly and see the change reflected in the
running container without a restart, or in combination with `docker compose watch`. Switching the default
would silently break those workflows, so nothing changes unless `driver: copy` is set. See
[docker/compose#11867](https://github.com/docker/compose/issues/11867) for the history of that decision.

Because the content is copied once, at container creation, editing the client's file afterwards has no effect
on already-running containers — recreate the service (`docker compose up`) to pick up a changed file, exactly
as for a secret declared with `content:`.

`driver: copy` requires `file` to be set. No other `driver` value is supported (as before, any other value is
rejected).
