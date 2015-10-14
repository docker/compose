import logging
import os

from docker import Client
from docker.utils import kwargs_from_env

from ..const import HTTP_TIMEOUT

log = logging.getLogger(__name__)


DEFAULT_API_VERSION = '1.19'


def docker_client(version=None):
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    if 'DOCKER_CLIENT_TIMEOUT' in os.environ:
        log.warn('The DOCKER_CLIENT_TIMEOUT environment variable is deprecated. Please use COMPOSE_HTTP_TIMEOUT instead.')

    kwargs = kwargs_from_env(assert_hostname=False)
    kwargs['version'] = version or os.environ.get(
        'COMPOSE_API_VERSION',
        DEFAULT_API_VERSION)
    kwargs['timeout'] = HTTP_TIMEOUT
    return Client(**kwargs)
