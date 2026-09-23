# compose-relay

`compose-relay` is the network gateway Docker Compose deploys **in place of a
provider-managed service**, so the other services of the project reach the
provider's resource at the compose-native address — `http://<service>:<port>` —
even though that resource lives outside the compose network.

## Why it exists

A service can delegate its implementation to an external
[provider](../docs/extension.md):

```yaml
services:
  web:
    build: .
    depends_on:
      - database

  database:
    provider:
      type: awesomecloud
```

The provider's resource (a cloud database, a sandbox, a host process, ...) is
not a container on the project network: `web` cannot resolve `database`, and
the resource's ports are typically published somewhere on the host, at
addresses and port numbers the application does not know. Until now consumers
had to read injected environment variables to locate it.

When the provider declares where each endpoint actually listens, with one
`publish-endpoint` message per port:

```json
{ "type": "publish-endpoint", "message": "5432=localhost:49152" }
```

Compose deploys this relay in place of the service. `web` then connects to
`database:5432` exactly as if the service were a regular container; the relay
forwards the connection to the real endpoint.

## How it runs

Compose creates the relay container from the published
[`docker/compose-relay`](https://hub.docker.com/r/docker/compose-relay) image
(override with `COMPOSE_RELAY_IMAGE`, e.g. for air-gapped setups or local
development) with:

- the service's canonical container name (`<project>-<service>-1`) and a
  network alias set to the service name, on the networks of every service
  that depends on the provider service;
- the standard compose labels, so label-driven commands (`ps`, `logs`,
  `stop`, `down`) treat it as the service — plus the
  `com.docker.compose.relay` label identifying its role. Its value is a hash
  of the routes, letting `up` keep an up-to-date relay and recreate a stale
  one. Commands that act on a service's process (`exec`, `cp`) refuse relay
  containers;
- the routes as environment:

  ```
  RELAY_ROUTES=5432=localhost:49152[,<port>=<host>:<port>...]
  ```

The binary listens on every declared container port and forwards each
connection to its endpoint. TCP only, with half-close propagation so
protocols relying on EOF work through the relay. It is intentionally minimal:
a static Go binary on a `scratch` image, no configuration reload — Compose
recreates the relay when the published endpoints change.

## Building

```console
$ docker buildx bake relay-image
```
