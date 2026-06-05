# CI relies on this ARG. Don't remove or rename it
ARG COMPOSE_VERSION=v5.1.4
FROM docker/compose-bin:${COMPOSE_VERSION} AS compose-bin


# DHI source: https://hub.docker.com/repository/docker/octopusdeploy/dhi-debian-base
FROM octopusdeploy/dhi-debian-base:trixie-debian13@sha256:436787c2d77ed1ef1cfe3ce5848f3968244d8948463a29094e1e672da9a6fa24 AS compose-plugin
WORKDIR /home/compose
COPY --chown=nonroot:nonroot --chmod=755 --from=compose-bin /docker-compose /usr/local/bin/docker-compose

ENV COMPOSE_COMPATIBILITY=true
USER nonroot:nonroot
ENTRYPOINT [ "docker-compose" ]
