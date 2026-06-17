# CI relies on this ARG. Don't remove or rename it
ARG COMPOSE_VERSION=v5.1.4
FROM docker/compose-bin:${COMPOSE_VERSION} AS compose-bin


# DHI source: https://hub.docker.com/repository/docker/octopusdeploy/dhi-debian-base
FROM octopusdeploy/dhi-debian-base:trixie-debian13@sha256:e440d0dabdc54675aa9601f0d794c39f549b6178946cfeffd3b5a31da33ec2d3 AS compose-plugin
WORKDIR /home/compose
COPY --chown=nonroot:nonroot --chmod=755 --from=compose-bin /docker-compose /usr/local/bin/docker-compose

ENV COMPOSE_COMPATIBILITY=true
USER nonroot:nonroot
ENTRYPOINT [ "docker-compose" ]
