FROM docker/compose-bin:v5.2.0@sha256:54c280c16d23289af63a9391626e3d09ddcd1253d4a5eef1f6ed52a531168e91 AS compose-bin


# DHI source: https://hub.docker.com/repository/docker/octopusdeploy/dhi-debian-base
FROM octopusdeploy/dhi-debian-base:trixie-debian13@sha256:53d4230bf16411bdc7d641665293ec06a0b58f528823f9e48db2929492be2f5c AS compose-plugin
WORKDIR /home/compose
COPY --chown=nonroot:nonroot --chmod=755 --from=compose-bin /docker-compose /usr/local/bin/docker-compose

ENV COMPOSE_COMPATIBILITY=true
USER nonroot:nonroot
ENTRYPOINT [ "docker-compose" ]
