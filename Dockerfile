# syntax=docker/dockerfile:1


ARG COMPOSE_VERSION=v2.32.2


FROM docker/compose-bin:${COMPOSE_VERSION} AS compose

FROM debian:bookworm-slim AS compose-plugin

COPY --from=compose /docker-compose /usr/local/bin/docker-compose
ENV COMPOSE_COMPATIBILITY=true

# add user
RUN addgroup --gid 3000 compose && \
  adduser --uid 3000 --gecos "" --disabled-password \
  --ingroup compose \
  --home /home/compose \
  --shell /bin/bash compose

WORKDIR /home/compose

RUN chown -R compose:compose /home/compose && \
  chmod 755 /home/compose

USER compose:compose

ENTRYPOINT [ "docker-compose" ]
