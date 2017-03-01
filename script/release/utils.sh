#!/bin/bash
#
# Util functions for release scripts
#

set -e
set -o pipefail


function browser() {
    local url=$1
    xdg-open $url || open $url
}


function find_remote() {
    local url=$1
    for remote in $(git remote); do
        git config --get remote.${remote}.url | grep $url > /dev/null && echo -n $remote
    done
    # Always return true, extra remotes cause it to return false
    true
}
