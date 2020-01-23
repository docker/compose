#!/bin/bash

set -e
set -x

## Usage :
## changelog PREVIOUS_TAG..HEAD

# configure refs so we get pull-requests metadata
git config --add remote.origin.fetch +refs/pull/*/head:refs/remotes/origin/pull/*
git fetch origin

RANGE=${1:-"$(git describe --tags  --abbrev=0 HEAD^)..HEAD"}
echo "Generate changelog for range ${RANGE}"
echo

pullrequests() {
    for commit in $(git log ${RANGE} --format='format:%H'); do
        # Get the oldest remotes/origin/pull/* branch to include this commit, i.e. the one to introduce it
        git branch -a --sort=committerdate  --contains $commit --list 'origin/pull/*' | head -1 | cut -d'/' -f4
    done
}

changes=$(pullrequests | uniq)

echo "pull requests merged within range:"
echo $changes

echo '#Features' > FEATURES.md
echo '#Bugs' > BUGS.md
for pr in $changes; do
    curl -fs -H "Authorization: token ${GITHUB_TOKEN}" https://api.github.com/repos/docker/compose/pulls/${pr} -o PR.json

    cat PR.json | jq -r ' select( .labels[].name | contains("kind/feature") ) | "- "+.title' >> FEATURES.md
    cat PR.json | jq -r ' select( .labels[].name | contains("kind/bug") ) | "- "+.title' >> BUGS.md
done

echo ${TAG_NAME} > CHANGELOG.md
echo >> CHANGELOG.md
cat FEATURES.md >> CHANGELOG.md
echo >> CHANGELOG.md
cat BUGS.md >> CHANGELOG.md
