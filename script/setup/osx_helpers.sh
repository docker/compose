#!/usr/bin/env bash

# Check file's ($1) SHA1 ($2).
check_sha1() {
  echo -n "$2 *$1" | shasum -c -
}

# Download URL ($1) to path ($2).
download() {
  curl -L $1 -o $2
}

# Extract tarball ($1) in folder ($2).
extract() {
  tar xf $1 -C $2
}

# Download URL ($1), check SHA1 ($3), and extract utility ($2).
fetch_tarball() {
  url=$1
  tarball=$2.tarball
  sha1=$3
  download $url $tarball
  check_sha1 $tarball $sha1
  extract $tarball $(dirname $tarball)
}

# Version of Python at toolchain path ($1).
python3_version() {
  $1/bin/python3 -V 2>&1
}

# Version of OpenSSL used by toolchain ($1) Python.
openssl_version() {
  $1/bin/python3 -c "import ssl; print(ssl.OPENSSL_VERSION)"
}

# System macOS version.
macos_version() {
  sw_vers -productVersion | cut -f1,2 -d'.'
}
