#!/bin/sh
set -ex

# import the existing docs build cmds from docker/docker
DOCSPORT=8000
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
DOCKER_DOCS_IMAGE="compose-docs$GIT_BRANCH"
DOCKER_RUN_DOCS="docker run --rm -it -e NOCACHE"

docker build -t "$DOCKER_DOCS_IMAGE" -f docs/Dockerfile .
$DOCKER_RUN_DOCS -p $DOCSPORT:8000 "$DOCKER_DOCS_IMAGE" mkdocs serve
