#!/bin/bash

if test -z $PYTHONBIN; then
  PYTHONBIN=$(which python3)
  if test -z $PYTHONBIN; then
    PYTHONBIN=$(which python)
  fi
fi

VERSION=$($PYTHONBIN -c "import sys; print('{}.{}'.format(*sys.version_info[0:2]))")
if test $(echo $VERSION | cut -d. -f1) -lt 3; then
  echo "Python 3.3 or above is required"
fi

if test $(echo $VERSION | cut -d. -f2) -lt 3; then
  echo "Python 3.3 or above is required"
fi

$PYTHONBIN -m venv ./.release-venv

VENVBINS=./.release-venv/bin

$VENVBINS/pip install -U Jinja2==2.10 \
    PyGithub==1.39 \
    pypandoc==1.4 \
    GitPython==2.1.9 \
    requests==2.18.4 \
    twine==1.11.0

$VENVBINS/python setup.py develop
