#!/bin/bash
#
# Util functions for release scritps
#

set -e


function browser() {
    local url=$1
    xdg-open $url || open $url
}


function find_remote() {
    local url=$1
    for remote in $(git remote); do
        git config --get remote.${remote}.url | grep $url > /dev/null && echo -n $remote
    done
}
