#!/bin/sh

if test -d ./.release-venv; then
    true
else
    ./script/release/setup-venv.sh
fi

args=$*

if test -z $args; then
    args="--help"
fi

./.release-venv/bin/python ./script/release/release.py $args
