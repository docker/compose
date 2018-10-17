#!/bin/sh

if test -d ${VENV_DIR:-./.release-venv}; then
    true
else
    ./script/release/setup-venv.sh
fi

if test -z "$*"; then
    args="--help"
fi

${VENV_DIR:-./.release-venv}/bin/python ./script/release/release.py "$@"
