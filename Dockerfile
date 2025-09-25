ARG COMPOSE_VERSION=v2.37.0
FROM docker/compose-bin:${COMPOSE_VERSION} AS compose

FROM debian:bookworm-20250908-slim AS compose-plugin
WORKDIR /home/compose
ENV COMPOSE_COMPATIBILITY=true
# add user
RUN addgroup --gid 3000 compose && \
  adduser --uid 3000 --gecos "" --disabled-password \
  --ingroup compose \
  --home /home/compose \
  --shell /bin/bash compose
USER compose:compose

COPY --from=compose --chown=compose:compose /docker-compose /usr/local/bin/docker-compose

ENTRYPOINT [ "docker-compose" ]
