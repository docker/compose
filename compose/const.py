from __future__ import absolute_import
from __future__ import unicode_literals

import sys

DEFAULT_TIMEOUT = 10
HTTP_TIMEOUT = 60
IMAGE_EVENTS = ['delete', 'import', 'load', 'pull', 'push', 'save', 'tag', 'untag']
IS_WINDOWS_PLATFORM = (sys.platform == "win32")
LABEL_CONTAINER_NUMBER = 'com.docker.compose.container-number'
LABEL_ONE_OFF = 'com.docker.compose.oneoff'
LABEL_PROJECT = 'com.docker.compose.project'
LABEL_SERVICE = 'com.docker.compose.service'
LABEL_NETWORK = 'com.docker.compose.network'
LABEL_VERSION = 'com.docker.compose.version'
LABEL_VOLUME = 'com.docker.compose.volume'
LABEL_CONFIG_HASH = 'com.docker.compose.config-hash'
NANOCPUS_SCALE = 1000000000

SECRETS_PATH = '/run/secrets'

COMPOSEFILE_V1 = '1'
COMPOSEFILE_V2_0 = '2.0'
COMPOSEFILE_V2_1 = '2.1'
COMPOSEFILE_V2_2 = '2.2'

COMPOSEFILE_V3_0 = '3.0'
COMPOSEFILE_V3_1 = '3.1'
COMPOSEFILE_V3_2 = '3.2'
COMPOSEFILE_V3_3 = '3.3'

API_VERSIONS = {
    COMPOSEFILE_V1: '1.21',
    COMPOSEFILE_V2_0: '1.22',
    COMPOSEFILE_V2_1: '1.24',
    COMPOSEFILE_V2_2: '1.25',
    COMPOSEFILE_V3_0: '1.25',
    COMPOSEFILE_V3_1: '1.25',
    COMPOSEFILE_V3_2: '1.25',
    COMPOSEFILE_V3_3: '1.30',
}

API_VERSION_TO_ENGINE_VERSION = {
    API_VERSIONS[COMPOSEFILE_V1]: '1.9.0',
    API_VERSIONS[COMPOSEFILE_V2_0]: '1.10.0',
    API_VERSIONS[COMPOSEFILE_V2_1]: '1.12.0',
    API_VERSIONS[COMPOSEFILE_V2_2]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_0]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_1]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_2]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_3]: '17.06.0',
}
