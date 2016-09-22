from __future__ import absolute_import
from __future__ import unicode_literals

import sys

DEFAULT_TIMEOUT = 10
HTTP_TIMEOUT = 60
IMAGE_EVENTS = ['delete', 'import', 'pull', 'push', 'tag', 'untag']
IS_WINDOWS_PLATFORM = (sys.platform == "win32")
LABEL_CONTAINER_NUMBER = 'com.docker.compose.container-number'
LABEL_ONE_OFF = 'com.docker.compose.oneoff'
LABEL_PROJECT = 'com.docker.compose.project'
LABEL_SERVICE = 'com.docker.compose.service'
LABEL_VERSION = 'com.docker.compose.version'
LABEL_CONFIG_HASH = 'com.docker.compose.config-hash'

COMPOSEFILE_V1 = '1'
COMPOSEFILE_V2_0 = '2.0'
COMPOSEFILE_V2_1 = '2.1'

API_VERSIONS = {
    COMPOSEFILE_V1: '1.21',
    COMPOSEFILE_V2_0: '1.22',
    COMPOSEFILE_V2_1: '1.24',
}

API_VERSION_TO_ENGINE_VERSION = {
    API_VERSIONS[COMPOSEFILE_V1]: '1.9.0',
    API_VERSIONS[COMPOSEFILE_V2_0]: '1.10.0',
    API_VERSIONS[COMPOSEFILE_V2_1]: '1.12.0',
}
