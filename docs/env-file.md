<!--[metadata]>
+++
title = "Environment file"
description = "Declaring default environment variables in file"
keywords = ["fig, composition, compose, docker, orchestration, environment, env file"]
[menu.main]
parent = "workw_compose"
weight=10
+++
<![end-metadata]-->


# Environment file

Compose supports declaring default environment variables in an environment
file named `.env` and placed in the same folder as your
[compose file](compose-file.md).

Compose expects each line in an env file to be in `VAR=VAL` format. Lines
beginning with `#` (i.e. comments) are ignored, as are blank lines.

> Note: Values present in the environment at runtime will always override
> those defined inside the `.env` file. Similarly, values passed via
> command-line arguments take precedence as well.

Those environment variables will be used for
[variable substitution](compose-file.md#variable-substitution) in your Compose
file, but can also be used to define the following
[CLI variables](reference/envvars.md):

- `COMPOSE_API_VERSION`
- `COMPOSE_FILE`
- `COMPOSE_HTTP_TIMEOUT`
- `COMPOSE_PROJECT_NAME`
- `DOCKER_CERT_PATH`
- `DOCKER_HOST`
- `DOCKER_TLS_VERIFY`

## More Compose documentation

- [User guide](index.md)
- [Command line reference](./reference/index.md)
- [Compose file reference](compose-file.md)
