

## Description

`docker compose convert` render the actual data model to be applied on target platform. When used with Docker engine,
it merges the Compose files set by `-f` flags, resolves variables in Compose file, and expands short-notation into 
fully defined Compose model. 

To allow smooth migration from docker-compose, this subcommand declares alias `docker compose config`
