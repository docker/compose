#!/bin/sh

if test -d ./.release-venv; then
    true
else
    ./script/release/setup-venv.sh
fi

if test -z "$*"; then
    args="--help"
fi

./.release-venv/bin/python ./script/release/release.py "$@"
