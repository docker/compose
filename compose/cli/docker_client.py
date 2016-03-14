from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os

from docker import Client
from docker.errors import TLSParameterError
from docker.utils import kwargs_from_env

from ..const import HTTP_TIMEOUT
from .errors import UserError

log = logging.getLogger(__name__)


def docker_client(version=None, tls_version=None):
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    if 'DOCKER_CLIENT_TIMEOUT' in os.environ:
        log.warn("The DOCKER_CLIENT_TIMEOUT environment variable is deprecated.  "
                 "Please use COMPOSE_HTTP_TIMEOUT instead.")

    try:
        kwargs = kwargs_from_env(assert_hostname=False, ssl_version=tls_version)
    except TLSParameterError:
        raise UserError(
            "TLS configuration is invalid - make sure your DOCKER_TLS_VERIFY "
            "and DOCKER_CERT_PATH are set correctly.\n"
            "You might need to run `eval \"$(docker-machine env default)\"`")

    if version:
        kwargs['version'] = version

    kwargs['timeout'] = HTTP_TIMEOUT

    return Client(**kwargs)
