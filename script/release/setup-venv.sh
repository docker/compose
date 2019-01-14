#!/bin/bash

debian_based() { test -f /etc/debian_version; }

if test -z $VENV_DIR; then
  VENV_DIR=./.release-venv
fi

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

# Debian / Ubuntu workaround:
# https://askubuntu.com/questions/879437/ensurepip-is-disabled-in-debian-ubuntu-for-the-system-python
if debian_based; then
  VENV_FLAGS="$VENV_FLAGS --without-pip"
fi

$PYTHONBIN -m venv $VENV_DIR $VENV_FLAGS

VENV_PYTHONBIN=$VENV_DIR/bin/python

if debian_based; then
  curl https://bootstrap.pypa.io/get-pip.py -o $VENV_DIR/get-pip.py
  $VENV_PYTHONBIN $VENV_DIR/get-pip.py
fi

$VENV_PYTHONBIN -m pip install -U Jinja2==2.10 \
    PyGithub==1.39 \
    GitPython==2.1.9 \
    requests==2.18.4 \
    setuptools==40.6.2 \
    twine==1.11.0

$VENV_PYTHONBIN setup.py develop
