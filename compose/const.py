import sys

from .version import ComposeVersion

DEFAULT_TIMEOUT = 10
HTTP_TIMEOUT = 60
IS_WINDOWS_PLATFORM = (sys.platform == "win32")
IS_LINUX_PLATFORM = (sys.platform == "linux")
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
COMPOSE_SPEC = ComposeVersion('3.9')

# minimum DOCKER ENGINE API version needed to support
# features for each compose schema version
API_VERSIONS = {
    COMPOSEFILE_V1: '1.21',
    COMPOSE_SPEC: '1.38',
}

API_VERSION_TO_ENGINE_VERSION = {
    API_VERSIONS[COMPOSEFILE_V1]: '1.9.0',
    API_VERSIONS[COMPOSE_SPEC]: '18.06.0',
}
