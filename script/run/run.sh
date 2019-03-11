#!/bin/sh -e
#
# Run docker-compose in a container
#
# This script will attempt to mirror the host paths by using volumes for the
# following paths:
#   * $(pwd)
#   * $(dirname $COMPOSE_FILE) if it's set
#   * $HOME if it's set
#
# You can add additional volumes (or any docker run options) using
# the $COMPOSE_OPTIONS environment variable.
#

VERSION=1.23.2
IMAGE="docker/compose:$VERSION"


# Setup options for connecting to docker host
[ -n "$DOCKER_HOST" ] || DOCKER_HOST=/var/run/docker.sock
if [ -S "$DOCKER_HOST" ]; then
	DOCKER_ADDR="-v $DOCKER_HOST:$DOCKER_HOST -e DOCKER_HOST"
else
	DOCKER_ADDR="-e DOCKER_HOST -e DOCKER_TLS_VERIFY -e DOCKER_CERT_PATH"
fi


# Setup volume mounts for compose config and context
[ "$(pwd)" == '/' ] || VOLUMES="-v $(pwd):$(pwd)"
[ -z "$COMPOSE_FILE" ] || {
	COMPOSE_OPTIONS="$COMPOSE_OPTIONS -e COMPOSE_FILE=$COMPOSE_FILE"
	compose_dir=$(realpath ${COMPOSE_FILE%/*})
}
# TODO: also check --file argument
[ -z "$compose_dir" ] || VOLUMES="$VOLUMES -v $compose_dir:$compose_dir"
[ -z "$HOME" ] || VOLUMES="$VOLUMES -v $HOME:$HOME -v $HOME:/root" # mount $HOME in /root to share docker.config

# Only allocate tty if we detect one
[ -t 0 ] && [ -t 1 ] && DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS -t"

# Always set -i to support piped and terminal input in run/exec
DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS -i"


# Handle userns security
[ -z "$(docker info 2>/dev/null | grep userns)" ] || DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS --userns=host"

exec docker run --rm $DOCKER_RUN_OPTIONS $DOCKER_ADDR $COMPOSE_OPTIONS $VOLUMES -w "$(pwd)" $IMAGE $*
