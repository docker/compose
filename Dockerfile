# CI relies on this ARG. Don't remove or rename it
ARG COMPOSE_VERSION=v5.1.1
FROM docker/compose-bin:${COMPOSE_VERSION} AS compose-bin


# DHI source: https://hub.docker.com/repository/docker/octopusdeploy/dhi-debian-base
FROM octopusdeploy/dhi-debian-base:trixie-debian13@sha256:75eec9a1a76a02ddf88761684efd9a72de6a5ec2c5d1abd002c03a475d5d23b7 AS compose-plugin
WORKDIR /home/compose
COPY --chown=nonroot:nonroot --chmod=755 --from=compose-bin /docker-compose /usr/local/bin/docker-compose

ENV COMPOSE_COMPATIBILITY=true
USER nonroot:nonroot
ENTRYPOINT [ "docker-compose" ]
