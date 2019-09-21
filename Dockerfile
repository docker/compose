ARG DOCKER_VERSION=18.09.7
ARG PYTHON_VERSION=3.7.4
ARG BUILD_ALPINE_VERSION=3.10
ARG BUILD_DEBIAN_VERSION=slim-stretch
ARG RUNTIME_ALPINE_VERSION=3.10.1
ARG RUNTIME_DEBIAN_VERSION=stretch-20190812-slim

ARG BUILD_PLATFORM=alpine

FROM docker:${DOCKER_VERSION} AS docker-cli

FROM python:${PYTHON_VERSION}-alpine${BUILD_ALPINE_VERSION} AS build-alpine
RUN apk add --no-cache \
    bash \
    build-base \
    ca-certificates \
    curl \
    gcc \
    git \
    libc-dev \
    libffi-dev \
    libgcc \
    make \
    musl-dev \
    openssl \
    openssl-dev \
    python2 \
    python2-dev \
    zlib-dev
ENV BUILD_BOOTLOADER=1

FROM python:${PYTHON_VERSION}-${BUILD_DEBIAN_VERSION} AS build-debian
RUN apt-get update && apt-get install --no-install-recommends -y \
    curl \
    gcc \
    git \
    libc-dev \
    libffi-dev \
    libgcc-6-dev \
    libssl-dev \
    make \
    openssl \
    python2.7-dev \
    zlib1g-dev

FROM build-${BUILD_PLATFORM} AS build
COPY docker-compose-entrypoint.sh /usr/local/bin/
ENTRYPOINT ["sh", "/usr/local/bin/docker-compose-entrypoint.sh"]
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
WORKDIR /code/
# FIXME(chris-crone): virtualenv 16.3.0 breaks build, force 16.2.0 until fixed
RUN pip install virtualenv==16.2.0
RUN pip install tox==2.9.1

COPY requirements.txt .
COPY requirements-dev.txt .
COPY .pre-commit-config.yaml .
COPY tox.ini .
COPY setup.py .
COPY README.md .
COPY compose compose/
RUN tox --notest
COPY . .
ARG GIT_COMMIT=unknown
ENV DOCKER_COMPOSE_GITSHA=$GIT_COMMIT
RUN script/build/linux-entrypoint

FROM alpine:${RUNTIME_ALPINE_VERSION} AS runtime-alpine
FROM debian:${RUNTIME_DEBIAN_VERSION} AS runtime-debian
FROM runtime-${BUILD_PLATFORM} AS runtime
COPY docker-compose-entrypoint.sh /usr/local/bin/
ENTRYPOINT ["sh", "/usr/local/bin/docker-compose-entrypoint.sh"]
COPY --from=docker-cli  /usr/local/bin/docker           /usr/local/bin/docker
COPY --from=build       /usr/local/bin/docker-compose   /usr/local/bin/docker-compose
