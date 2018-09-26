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

if test -z $(python -c "import docker; print(docker.version)" 2>/dev/null); then
    echo "This script requires the 'docker' Python package to be installed locally"
    exit 1
fi

hub_credentials=$(python -c "from docker import auth; cfg = auth.load_config(); print(auth.encode_header(auth.resolve_authconfig(cfg, 'docker.io')).decode('ascii'))")

docker run -it \
    -e GITHUB_TOKEN=$GITHUB_TOKEN \
    -e BINTRAY_TOKEN=$BINTRAY_TOKEN \
    -e SSH_AUTH_SOCK=$SSH_AUTH_SOCK \
    -e HUB_CREDENTIALS=$hub_credentials \
    --mount type=bind,source=$(pwd),target=/src \
    --mount type=bind,source=$HOME/.gitconfig,target=/root/.gitconfig \
    --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
    --mount type=bind,source=$HOME/.ssh,target=/root/.ssh \
    --mount type=bind,source=/tmp,target=/tmp \
    -v $HOME/.pypirc:/root/.pypirc \
    compose/release-tool $*
