#!/bin/bash

curl -f -u$BINTRAY_USERNAME:$BINTRAY_API_KEY -X GET \
  https://api.bintray.com/repos/docker-compose/${CIRCLE_BRANCH}

if test $? -ne 0; then
  echo "Bintray repository ${CIRCLE_BRANCH} does not exist ; abandoning upload attempt"
  exit 0
fi

curl -u$BINTRAY_USERNAME:$BINTRAY_API_KEY -X POST \
  -d "{\
    \"name\": \"${PKG_NAME}\", \"desc\": \"auto\", \"licenses\": [\"Apache-2.0\"], \
    \"vcs_url\": \"${CIRCLE_REPOSITORY_URL}\" \
  }" -H "Content-Type: application/json" \
  https://api.bintray.com/packages/docker-compose/${CIRCLE_BRANCH}

curl -u$BINTRAY_USERNAME:$BINTRAY_API_KEY -X POST -d "{\
    \"name\": \"$CIRCLE_BRANCH\", \
    \"desc\": \"Automated build of the ${CIRCLE_BRANCH} branch.\", \
  }" -H "Content-Type: application/json" \
  https://api.bintray.com/packages/docker-compose/${CIRCLE_BRANCH}/${PKG_NAME}/versions

curl -f -T dist/docker-compose-${OS_NAME}-x86_64 -u$BINTRAY_USERNAME:$BINTRAY_API_KEY \
  -H "X-Bintray-Package: ${PKG_NAME}" -H "X-Bintray-Version: $CIRCLE_BRANCH" \
  -H "X-Bintray-Override: 1" -H "X-Bintray-Publish: 1" -X PUT \
  https://api.bintray.com/content/docker-compose/${CIRCLE_BRANCH}/docker-compose-${OS_NAME}-x86_64 || exit 1
