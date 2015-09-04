import os

from docker import Client
from docker.utils import kwargs_from_env


def docker_client():
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    kwargs = kwargs_from_env(assert_hostname=False)
    kwargs['version'] = os.environ.get('COMPOSE_API_VERSION', '1.19')
    kwargs['timeout'] = int(os.environ.get('DOCKER_CLIENT_TIMEOUT', 60))
    return Client(**kwargs)
