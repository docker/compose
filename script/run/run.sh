#!/bin/sh
#
# Run docker-compose in a container
#
# This script will attempt to mirror the host paths by using volumes for the
# following paths:
#   * ${PWD}
#   * $(dirname ${COMPOSE_FILE}) if it's set
#   * ${HOME} if it's set
#
# You can add additional volumes (or any docker run options) using
# the ${COMPOSE_OPTIONS} environment variable.
#
# You can set a specific image and tag, such as "docker/compose:1.27.4", or "docker/compose:alpine-1.27.4"
# using the $DOCKER_COMPOSE_IMAGE_TAG environment variable (defaults to "docker/compose:1.27.4")
#

set -e

# Setup options for connecting to docker host
if [ -z "${DOCKER_HOST}" ]; then
    DOCKER_HOST='unix:///var/run/docker.sock'
fi
if [ -S "${DOCKER_HOST#unix://}" ]; then
    DOCKER_ADDR="-v ${DOCKER_HOST#unix://}:${DOCKER_HOST#unix://} -e DOCKER_HOST"
else
    DOCKER_ADDR="-e DOCKER_HOST -e DOCKER_TLS_VERIFY -e DOCKER_CERT_PATH"
fi

# Setup volume mounts for compose config and context
if [ "${PWD}" != '/' ]; then
    VOLUMES="-v ${PWD}:${PWD}"
fi
if [ -n "${COMPOSE_FILE}" ]; then
    COMPOSE_OPTIONS="${COMPOSE_OPTIONS} -e COMPOSE_FILE=${COMPOSE_FILE}"
    COMPOSE_DIR="$(dirname "${COMPOSE_FILE}")"
    # canonicalize dir, do not use realpath or readlink -f
    # since they are not available in some systems (e.g. macOS).
    COMPOSE_DIR="$(cd "${COMPOSE_DIR}" && pwd)"
fi
if [ -n "${COMPOSE_PROJECT_NAME}" ]; then
    COMPOSE_OPTIONS="-e COMPOSE_PROJECT_NAME ${COMPOSE_OPTIONS}"
fi
# TODO: also check --file argument
if [ -n "${COMPOSE_DIR}" ]; then
    VOLUMES="${VOLUMES} -v ${COMPOSE_DIR}:${COMPOSE_DIR}"
fi
if [ -n "${HOME}" ]; then
    VOLUMES="${VOLUMES} -v ${HOME}:${HOME} -e HOME" # Pass in HOME to share docker.config and allow ~/-relative paths to work.
fi

# Only allocate tty if we detect one
if [ -t 0 ] && [ -t 1 ]; then
    DOCKER_RUN_OPTIONS="${DOCKER_RUN_OPTIONS} -t"
fi

# Always set -i to support piped and terminal input in run/exec
DOCKER_RUN_OPTIONS="${DOCKER_RUN_OPTIONS} -i"

# Handle userns security
if docker info --format '{{json .SecurityOptions}}' 2> /dev/null | grep -q 'name=userns'; then
    DOCKER_RUN_OPTIONS="${DOCKER_RUN_OPTIONS} --userns=host"
fi

# Detect SELinux and add --privileged if necessary
if docker info --format '{{json .SecurityOptions}}' 2> /dev/null | grep -q 'name=selinux'; then
    DOCKER_RUN_OPTIONS="${DOCKER_RUN_OPTIONS} --privileged"
fi

# shellcheck disable=SC2086
exec docker run --rm ${DOCKER_RUN_OPTIONS} ${DOCKER_ADDR} ${COMPOSE_OPTIONS} ${VOLUMES} -w "${PWD}" "${DOCKER_COMPOSE_IMAGE_TAG:-docker/compose:1.27.4}" "$@"
