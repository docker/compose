# Per-replica volumes

## Problem

When a service is scaled to several replicas (`deploy.replicas` or `--scale`), every replica shares the exact
same named volume:

```yaml
services:
  configsvr:
    image: mongo
    command: mongod --configsvr --replSet configsvr
    deploy:
      replicas: 3
    volumes:
      - configsvr-data:/data/db

volumes:
  configsvr-data:
```

All three `configsvr` replicas above mount the very same `configsvr-data` volume, which corrupts any service
that expects each replica to keep its own independent state (e.g. members of a database replica set). The only
workaround has been to hand-declare one service per replica, each with its own named volume — defeating the
purpose of `replicas`.

## `x-per-replica`

Setting `x-per-replica: true` on a long-syntax volume entry gives every replica of that service its own,
distinct volume instead of sharing the one declared under the top-level `volumes:` key:

```yaml
services:
  configsvr:
    image: mongo
    command: mongod --configsvr --replSet configsvr
    deploy:
      replicas: 3
    volumes:
      - type: volume
        source: configsvr-data
        target: /data/db
        x-per-replica: true

volumes:
  configsvr-data:
```

Replica `N` of `configsvr` gets a volume named `<project>_configsvr-data-N` (`myproject_configsvr-data-1`,
`myproject_configsvr-data-2`, `myproject_configsvr-data-3`), created on demand with the same driver, driver
options and labels as the declared `configsvr-data` volume. Replicas keep the same volume across restarts and
recreation, exactly like a normal named volume; only the sharing across replicas is removed.

`docker compose down --volumes` removes every per-replica volume actually backing the project's containers,
alongside the (in this mode, never populated) base volume name.

`x-per-replica` requires a named volume (`type: volume` with a `source`) and cannot be combined with
`external: true` — Compose only mints new volume names for volumes it owns.

`x-per-replica` is a Compose-specific extension: it is not (yet) part of the
[Compose Specification](https://github.com/compose-spec/compose-spec), so other tools that consume Compose
files won't recognize it.
