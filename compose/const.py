from .validators import service_schema

# service configuration

ALLOWED_KEYS = set(service_schema.keys())

DOCKER_CONFIG_KEYS = ALLOWED_KEYS.copy()
DOCKER_CONFIG_KEYS |= set(['detach'])
DOCKER_CONFIG_KEYS -= set(['build', 'external_links', 'expose'])

# labels

LABEL_CONTAINER_NUMBER = 'com.docker.compose.container-number'
LABEL_ONE_OFF = 'com.docker.compose.oneoff'
LABEL_PROJECT = 'com.docker.compose.project'
LABEL_SERVICE = 'com.docker.compose.service'
LABEL_VERSION = 'com.docker.compose.version'
LABEL_CONFIG_HASH = 'com.docker.compose.config-hash'
