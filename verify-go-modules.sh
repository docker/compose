#!/usr/bin/env bash

#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

#
# verifies if the require and replace directives for two go.mod files are in sync
#
set -eu -o pipefail

ROOT=$(dirname "${BASH_SOURCE}")

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 dir-for-second-go-mod"
  exit 1
fi

if ! command -v jq &> /dev/null ; then
  echo Please install jq
  exit 1
fi

# Load the requires and replaces section in the root go.mod file
declare -A map_requires_1
declare -A map_replaces_1
pushd "${ROOT}" > /dev/null
while IFS='#' read -r key value
do
  map_requires_1[$key]="$value"
done<<<$(go mod edit -json | jq -r '.Require[] | .Path +  " # " + .Version')
while IFS='#' read -r key value
do
  [ "$key" = "" ] || map_replaces_1[$key]="$value"
done<<<$(go mod edit -json | jq -r 'try .Replace[] | .Old.Path + " # " + .New.Path + " : " + .New.Version')
popd > /dev/null

# Load the requires and replaces section in the other go.mod file
declare -A map_requires_2
declare -A map_replaces_2
pushd "${ROOT}/$1" > /dev/null
while IFS='#' read -r key value
do
  [ "$key" = "" ] || map_requires_2[$key]="$value"
done<<<$(go mod edit -json | jq -r '.Require[] | .Path +  " # " + .Version')
while IFS='#' read -r key value
do
  map_replaces_2[$key]="$value"
done<<<$(go mod edit -json | jq -r 'try .Replace[] | .Old.Path + " # " + .New.Path + " : " + .New.Version')
popd > /dev/null

# signal for errors later
ERRORS=0

# iterate through the second go.mod's require section and ensure that all items
# have the same values in the root go.mod replace section
for k in "${!map_requires_2[@]}"
do
  if [ -v "map_requires_1[$k]" ]; then
    if [ "${map_requires_2[$k]}" != "${map_requires_1[$k]}" ]; then
      echo "${k} has different values in the go.mod files require section:" \
        "${map_requires_1[$k]} in root go.mod ${map_requires_2[$k]} in $1/go.mod"
      ERRORS=$(( ERRORS + 1 ))
    fi
  fi
done

# iterate through the second go.mod's replace section and ensure that all items
# have the same values in the root go.mod's replace section. Except for the
# containerd/containerd which we know will be different
for k in "${!map_replaces_2[@]}"
do
  if [[ "${k}" == "github.com/docker/compose"* ]]; then
    continue
  fi
  if [ -v "map_replaces_1[$k]" ]; then
    if [ "${map_replaces_2[$k]}" != "${map_replaces_1[$k]}" ]; then
      echo "${k} has different values in the go.mod files replace section:" \
        "${map_replaces_1[$k]} in root go.mod ${map_replaces_2[$k]} in $1/go.mod"
      ERRORS=$(( ERRORS + 1 ))
    fi
  fi
done

# iterate through the root go.mod's replace section and ensure that all the
# same items are present in the second go.mod's replace section and nothing is missing
for k in "${!map_replaces_1[@]}"
do
  if [[ "${k}" == "github.com/docker/compose"* ]]; then
    continue
  fi
  if [ ! -v "map_replaces_2[$k]" ]; then
    echo "${k} has an entry in root go.mod replace section, but is missing from" \
      " replace section in $1/go.mod"
    ERRORS=$(( ERRORS + 1 ))
  fi
done

if [ "$ERRORS" -ne 0 ]; then
  echo "Found $ERRORS error(s)."
  exit 1
fi
