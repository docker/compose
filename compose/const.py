from __future__ import absolute_import
from __future__ import unicode_literals

import sys

from .version import ComposeVersion

DEFAULT_TIMEOUT = 10
HTTP_TIMEOUT = 60
IS_WINDOWS_PLATFORM = (sys.platform == "win32")
LABEL_CONTAINER_NUMBER = 'com.docker.compose.container-number'
LABEL_ONE_OFF = 'com.docker.compose.oneoff'
LABEL_PROJECT = 'com.docker.compose.project'
LABEL_WORKING_DIR = 'com.docker.compose.project.working_dir'
LABEL_CONFIG_FILES = 'com.docker.compose.project.config_files'
LABEL_ENVIRONMENT_FILE = 'com.docker.compose.project.environment_file'
LABEL_SERVICE = 'com.docker.compose.service'
LABEL_NETWORK = 'com.docker.compose.network'
LABEL_VERSION = 'com.docker.compose.version'
LABEL_SLUG = 'com.docker.compose.slug'
LABEL_VOLUME = 'com.docker.compose.volume'
LABEL_CONFIG_HASH = 'com.docker.compose.config-hash'
NANOCPUS_SCALE = 1000000000
PARALLEL_LIMIT = 64

SECRETS_PATH = '/run/secrets'
WINDOWS_LONGPATH_PREFIX = '\\\\?\\'

COMPOSEFILE_V1 = ComposeVersion('1')
COMPOSEFILE_V2_0 = ComposeVersion('2.0')
COMPOSEFILE_V2_1 = ComposeVersion('2.1')
COMPOSEFILE_V2_2 = ComposeVersion('2.2')
COMPOSEFILE_V2_3 = ComposeVersion('2.3')
COMPOSEFILE_V2_4 = ComposeVersion('2.4')

COMPOSEFILE_V3_0 = ComposeVersion('3.0')
COMPOSEFILE_V3_1 = ComposeVersion('3.1')
COMPOSEFILE_V3_2 = ComposeVersion('3.2')
COMPOSEFILE_V3_3 = ComposeVersion('3.3')
COMPOSEFILE_V3_4 = ComposeVersion('3.4')
COMPOSEFILE_V3_5 = ComposeVersion('3.5')
COMPOSEFILE_V3_6 = ComposeVersion('3.6')
COMPOSEFILE_V3_7 = ComposeVersion('3.7')

API_VERSIONS = {
    COMPOSEFILE_V1: '1.21',
    COMPOSEFILE_V2_0: '1.22',
    COMPOSEFILE_V2_1: '1.24',
    COMPOSEFILE_V2_2: '1.25',
    COMPOSEFILE_V2_3: '1.30',
    COMPOSEFILE_V2_4: '1.35',
    COMPOSEFILE_V3_0: '1.25',
    COMPOSEFILE_V3_1: '1.25',
    COMPOSEFILE_V3_2: '1.25',
    COMPOSEFILE_V3_3: '1.30',
    COMPOSEFILE_V3_4: '1.30',
    COMPOSEFILE_V3_5: '1.30',
    COMPOSEFILE_V3_6: '1.36',
    COMPOSEFILE_V3_7: '1.38',
}

API_VERSION_TO_ENGINE_VERSION = {
    API_VERSIONS[COMPOSEFILE_V1]: '1.9.0',
    API_VERSIONS[COMPOSEFILE_V2_0]: '1.10.0',
    API_VERSIONS[COMPOSEFILE_V2_1]: '1.12.0',
    API_VERSIONS[COMPOSEFILE_V2_2]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V2_3]: '17.06.0',
    API_VERSIONS[COMPOSEFILE_V2_4]: '17.12.0',
    API_VERSIONS[COMPOSEFILE_V3_0]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_1]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_2]: '1.13.0',
    API_VERSIONS[COMPOSEFILE_V3_3]: '17.06.0',
    API_VERSIONS[COMPOSEFILE_V3_4]: '17.06.0',
    API_VERSIONS[COMPOSEFILE_V3_5]: '17.06.0',
    API_VERSIONS[COMPOSEFILE_V3_6]: '18.02.0',
    API_VERSIONS[COMPOSEFILE_V3_7]: '18.06.0',
}
