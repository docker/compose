#!/bin/sh
#!/bin/sh -x
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

VERSION="1.26.1"
IMAGE="docker/compose:$VERSION"

DOCKER_HOST_REMOTE='unix:///var/run/docker.sock'

# Setup options for connecting to docker host
if [ -z "$DOCKER_HOST" ]; then
    DOCKER_HOST="${DOCKER_HOST_REMOTE}"
fi
if [ -S "${DOCKER_HOST#unix://}" ]; then
    DOCKER_ADDR="--volume ${DOCKER_HOST#unix://}:${DOCKER_HOST#unix://} --env DOCKER_HOST"
else
    # shellcheck disable=SC2046,SC2086
    if [ 0 -eq $( \
      docker run --rm $DOCKER_ADDR \
        --volume ${DOCKER_HOST_REMOTE#unix://}:${DOCKER_HOST_REMOTE#unix://} \
        --env DOCKER_HOST=${DOCKER_HOST_REMOTE} \
        --entrypoint=/bin/sh \
        $IMAGE \
        -c 'if test -S "${DOCKER_HOST#unix://}" ; then echo 0; else echo 1; fi ;' \
    ) ]; then
      DOCKER_ADDR="--volume ${DOCKER_HOST_REMOTE#unix://}:${DOCKER_HOST_REMOTE#unix://} --env DOCKER_HOST=${DOCKER_HOST_REMOTE}"
    else
      DOCKER_ADDR="--env DOCKER_HOST --env DOCKER_TLS_VERIFY --env DOCKER_CERT_PATH"
    fi
fi


# Setup volume mounts for compose config and context
if [ "$(pwd)" != '/' ]; then
    VOLUMES="--volume $(pwd):$(pwd)"
fi
if [ -n "$COMPOSE_FILE" ]; then
    COMPOSE_OPTIONS="$COMPOSE_OPTIONS -e COMPOSE_FILE=$COMPOSE_FILE"
    compose_dir="$(dirname "$COMPOSE_FILE")"
    # canonicalize dir, do not use realpath or readlink -f
    # since they are not available in some systems (e.g. macOS).
    compose_dir="$(cd "$compose_dir" && pwd)"
fi
if [ -n "$COMPOSE_PROJECT_NAME" ]; then
    COMPOSE_OPTIONS="-e COMPOSE_PROJECT_NAME $COMPOSE_OPTIONS"
fi
if [ -n "$compose_dir" ]; then
    VOLUMES="$VOLUMES --volume $compose_dir:$compose_dir"
fi
if [ -n "$HOME" ]; then
    VOLUMES="$VOLUMES --volume $HOME:$HOME --env HOME" # Pass in HOME to share docker.config and allow ~/-relative paths to work.
fi
i=$#
# shellcheck disable=SC2086
while [ $i -gt 0 ]; do
    arg=$1
    i=$((i - 1))
    shift

    case "$arg" in
        -f|--file)
            value=$1
            i=$((i - 1))
            shift
            set -- "$@" "$arg" "$value"

            file_dir=$(realpath "$(dirname "$value")")
            VOLUMES="$VOLUMES --volume $file_dir:$file_dir"
        ;;
        *) set -- "$@" "$arg" ;;
    esac
done

# Setup environment variables for compose config and context
# shellcheck disable=SC1117
ENV_OPTIONS=$(printenv | sed -E "/^PATH=.*/d; /^DOCKER_HOST=.*/d; s/^/-e /g; s/=.*//g; s/\n/ /g")

# Only allocate tty if we detect one
if [ -t 0 ] && [ -t 1 ]; then
    DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS --tty"
fi

# Always set -i to support piped and terminal input in run/exec
DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS --interactive"


# Handle userns security
if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -q 'name=userns'; then
    DOCKER_RUN_OPTIONS="$DOCKER_RUN_OPTIONS --userns=host"
fi

# shellcheck disable=SC2086
#exec docker -vvv run --rm $DOCKER_RUN_OPTIONS $DOCKER_ADDR $COMPOSE_OPTIONS $ENV_OPTIONS $VOLUMES -w "$(pwd)" $IMAGE --verbose "$@"
exec docker run --rm $DOCKER_RUN_OPTIONS $DOCKER_ADDR $COMPOSE_OPTIONS $ENV_OPTIONS $VOLUMES -w "$(pwd)" $IMAGE "$@"
