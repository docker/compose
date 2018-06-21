#!/bin/sh

docker image inspect compose/release-tool > /dev/null
if test $? -ne 0; then
    docker build -t compose/release-tool -f $(pwd)/script/release/Dockerfile $(pwd)
fi

if test -z $GITHUB_TOKEN; then
    echo "GITHUB_TOKEN environment variable must be set"
    exit 1
fi

if test -z $BINTRAY_TOKEN; then
    echo "BINTRAY_TOKEN environment variable must be set"
    exit 1
fi

docker run -e GITHUB_TOKEN=$GITHUB_TOKEN -e BINTRAY_TOKEN=$BINTRAY_TOKEN -e SSH_AUTH_SOCK=$SSH_AUTH_SOCK -it \
    --mount type=bind,source=$(pwd),target=/src \
    --mount type=bind,source=$HOME/.docker,target=/root/.docker \
    --mount type=bind,source=$HOME/.gitconfig,target=/root/.gitconfig \
    --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
    --mount type=bind,source=$HOME/.ssh,target=/root/.ssh \
    --mount type=bind,source=/tmp,target=/tmp \
    -v $HOME/.pypirc:/root/.pypirc \
    compose/release-tool $*
