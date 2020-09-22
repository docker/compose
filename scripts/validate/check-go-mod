#!/bin/sh

#   Copyright Docker Compose CLI authors

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.


set -uo pipefail
mkdir -p /tmp/gomod
cp go.* /tmp/gomod/
go mod tidy
DIFF=$(diff go.mod /tmp/gomod/go.mod && diff go.sum /tmp/gomod/go.sum)
if [ "$DIFF" ]; then
    echo
    echo "go.mod and go.sum are not up to date"
    echo
    echo "$DIFF"
    echo
    exit 1
else
    echo "go.mod is correct"
fi;
