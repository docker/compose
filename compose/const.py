from __future__ import absolute_import
from __future__ import unicode_literals

import os
import sys

DEFAULT_TIMEOUT = 10
HTTP_TIMEOUT = int(os.environ.get('COMPOSE_HTTP_TIMEOUT', os.environ.get('DOCKER_CLIENT_TIMEOUT', 60)))
IMAGE_EVENTS = ['delete', 'import', 'pull', 'push', 'tag', 'untag']
IS_WINDOWS_PLATFORM = (sys.platform == "win32")
LABEL_CONTAINER_NUMBER = 'com.docker.compose.container-number'
LABEL_ONE_OFF = 'com.docker.compose.oneoff'
LABEL_PROJECT = 'com.docker.compose.project'
LABEL_SERVICE = 'com.docker.compose.service'
LABEL_VERSION = 'com.docker.compose.version'
LABEL_CONFIG_HASH = 'com.docker.compose.config-hash'
COMPOSEFILE_VERSIONS = (1, 2)

API_VERSIONS = {
    1: '1.21',

    # TODO: update to 1.22 when there's a Docker 1.10 build to test against
    2: '1.21',
}
