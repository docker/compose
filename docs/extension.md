# About

The Compose application model defines `service` as an abstraction for a computing unit managing (a subset of)
application needs, which can interact with other service by relying on network(s). Docker Compose is designed 
to use the Docker Engine ("Moby") API to manage services as containers, but the abstraction _could_ also cover 
many other runtimes, typically cloud services or services natively provided by host.

The Compose extensibility model has been designed to extend the `service` support to runtimes accessible through
third-party tooling.

# Architecture

Compose extensibility relies on the `provider` attribute to select the actual binary responsible for managing
the resource(s) needed to run a service.

```yaml
  database:
    provider:
      type: awesomecloud
      options:
        type: mysql
        size: 256
```

`provider.type` tells Compose the binary to run, which can be either:
- Another Docker CLI plugin (typically, `model` to run `docker-model`)
- An executable in user's `PATH`

If `provider.type` doesn't resolve into any of those, Compose will report an error and interrupt the `up` command.

To be a valid Compose extension, provider command *MUST* accept a `compose` command (which can be hidden)
with subcommands `up` and `down`.

## Up lifecycle

To execute an application's `up` lifecycle, Compose executes the provider's `compose up` command, passing 
the project name, service name, and additional options. The `provider.options` are translated 
into command line flags. For example:
```console
awesomecloud compose --project-name <NAME> up --type=mysql --size=256 "database"
```

> __Note:__ `project-name` _should_ be used by the provider to tag resources
> set for project, so that later execution with `down` subcommand releases 
> all allocated resources set for the project.

## Communication with Compose

Providers can interact with Compose using `stdout` as a channel, sending JSON line delimited messages.
JSON messages MUST include a `type` and a `message` attribute.
```json
{ "type": "info", "message": "preparing mysql ..." }
```

`type` can be either:
- `info`: Reports status updates to the user. Compose will render message as the service state in the progress UI
- `error`: Lest the user know something went wrong with details about the error. Compose will render the message as the reason for the service failure.
- `setenv`: Let's the plugin tell Compose how dependent services can access the created resource. See next section for further details.

```mermaid
sequenceDiagram
    Shell->>Compose: docker compose up
    Compose->>Provider: compose up --project-name=xx --foo=bar "database"
    Provider--)Compose: json { "info": "pulling 25%" }
    Compose-)Shell: pulling 25%
    Provider--)Compose: json { "info": "pulling 50%" }
    Compose-)Shell: pulling 50%
    Provider--)Compose: json { "info": "pulling 75%" }
    Compose-)Shell: pulling 75%
    Provider--)Compose: json { "setenv": "URL=http://cloud.com/abcd:1234" }
    Compose-)Compose: set DATABASE_URL
    Provider-)Compose: EOF (command complete) exit 0
    Compose-)Shell: service started
```

## Connection to a service managed by a provider

A service in the Compose application can declare dependency on a service managed by an external provider: 

```yaml
services:
  app:
    image: myapp 
    depends_on:
       - database

  database:
    provider:
      type: awesomecloud
```

When the provider command sends a `setenv` JSON message, Compose injects the specified variable into any dependent service,
automatically prefixing it with the service name. For example, if `awesomecloud compose up` returns:
```json
{"type": "setenv", "message": "URL=https://awesomecloud.com/db:1234"}
```
Then the `app` service, which depends on the service managed by the provider, will receive a `DATABASE_URL` environment variable injected
into its runtime environment.

> __Note:__  The `compose up` provider command _MUST_ be idempotent. If resource is already running, the command _MUST_ set
> the same environment variables to ensure consistent configuration of dependent services.

## Down lifecycle

`down` lifecycle is equivalent to `up` with the `<provider> compose --project-name <NAME> down <SERVICE>` command.
The provider is responsible for releasing all resources associated with the service. 