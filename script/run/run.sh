#!/bin/sh
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


set -e

VERSION="1.25.0"
IMAGE="docker/compose:$VERSION"


# Setup options for connecting to docker host
if [ -z "$DOCKER_HOST" ]; then
    DOCKER_HOST="/var/run/docker.sock"
fi
if [ -S "$DOCKER_HOST" ]; then
    DOCKER_ADDR="-v $DOCKER_HOST:$DOCKER_HOST -e DOCKER_HOST"
else
    DOCKER_ADDR="-e DOCKER_HOST -e DOCKER_TLS_VERIFY -e DOCKER_CERT_PATH"
fi


# Setup volume mounts for compose config and context
if [ "$(pwd)" != '/' ]; then
    VOLUMES="-v $(pwd):$(pwd)"
fi
if [ -z "$COMPOSE_FILE" ] ; then
    # Iterate over $@ in a subshell, to not alter the original
    COMPOSE_FILE=$(
    while [ "$#" -gt 0 ] ; do
        case "$1" in
            -f|--file)
                if [ -n "$2" ] ; then
                    COMPOSE_FILE=$2
                    shift
                fi
                ;;
            --file=*)
                COMPOSE_FILE=${1#--file=}
                ;;
            build|bundle|config|create|down|events|exec|help|images|kill|logs|pause|port|ps|pull|push|restart|rm|run|scale|start|stop|top|unpause|up|version)
                # Break on any docker-compose verb,
                # because any -f or --file after that
                # isn't the droid we're looking for.
                break
                ;;
        esac
        shift
    done
    echo "$COMPOSE_FILE"
)
fi
if [ -n "$COMPOSE_FILE" ]; then
    if [ ! -s "${COMPOSE_FILE}" ]; then
        echo "$0: Compose-file ${COMPOSE_FILE} does not exist, or is empty"
        exit 1
    fi
    COMPOSE_OPTIONS="$COMPOSE_OPTIONS -e COMPOSE_FILE=$COMPOSE_FILE"
    compose_dir=$(realpath "$(dirname "$COMPOSE_FILE")")
fi
if [ -n "$compose_dir" ]; then
    VOLUMES="$VOLUMES -v $compose_dir:$compose_dir"
fi
if [ -n "$HOME" ]; then
    VOLUMES="$VOLUMES -v $HOME:$HOME -e HOME" # Pass in HOME to share docker.config and allow ~/-relative paths to work.
fi

# Only allocate tty if we detect one
if [ -t 0 ] && [ -t 1 ]; then
    DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS -t"
fi

# Always set -i to support piped and terminal input in run/exec
DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS -i"


# Handle userns security
if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -q 'name=userns'; then
    DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS --userns=host"
fi

# shellcheck disable=SC2086
exec docker run --rm $DOCKER_RUN_OPTIONS $DOCKER_ADDR $COMPOSE_OPTIONS $VOLUMES -w "$(pwd)" $IMAGE "$@"
