# CI relies on this ARG. Don't remove or rename it
ARG COMPOSE_VERSION=v5.1.3
FROM docker/compose-bin:${COMPOSE_VERSION} AS compose-bin


# DHI source: https://hub.docker.com/repository/docker/octopusdeploy/dhi-debian-base
FROM octopusdeploy/dhi-debian-base:trixie-debian13@sha256:79ea7f22d1b7e3f73b0988258b62bcbf73da44f0d82476fbb95d811130168e55 AS compose-plugin
WORKDIR /home/compose
COPY --chown=nonroot:nonroot --chmod=755 --from=compose-bin /docker-compose /usr/local/bin/docker-compose

ENV COMPOSE_COMPATIBILITY=true
USER nonroot:nonroot
ENTRYPOINT [ "docker-compose" ]
