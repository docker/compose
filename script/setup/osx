#!/bin/bash

set -ex

python_version() {
  python -V 2>&1
}

openssl_version() {
  python -c "import ssl; print ssl.OPENSSL_VERSION"
}

desired_python_version="2.7.12"
desired_python_brew_version="2.7.12"
python_formula="https://raw.githubusercontent.com/Homebrew/homebrew-core/737a2e34a89b213c1f0a2a24fc1a3c06635eed04/Formula/python.rb"

desired_openssl_version="1.0.2j"
desired_openssl_brew_version="1.0.2j"
openssl_formula="https://raw.githubusercontent.com/Homebrew/homebrew-core/30d3766453347f6e22b3ed6c74bb926d6def2eb5/Formula/openssl.rb"

PATH="/usr/local/bin:$PATH"

if !(which brew); then
  ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
fi

brew update > /dev/null

if !(python_version | grep "$desired_python_version"); then
  if brew list | grep python; then
    brew unlink python
  fi

  brew install "$python_formula"
  brew switch python "$desired_python_brew_version"
fi

if !(openssl_version | grep "$desired_openssl_version"); then
  if brew list | grep openssl; then
    brew unlink openssl
  fi

  brew install "$openssl_formula"
  brew switch openssl "$desired_openssl_brew_version"
fi

echo "*** Using $(python_version)"
echo "*** Using $(openssl_version)"

if !(which virtualenv); then
  pip install virtualenv
fi
