from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from docker import Client
from docker.errors import TLSParameterError
from docker.tls import TLSConfig
from docker.utils import kwargs_from_env

from ..const import HTTP_TIMEOUT
from .errors import UserError
from .utils import generate_user_agent
from .utils import unquote_path

log = logging.getLogger(__name__)


def tls_config_from_options(options):
    tls = options.get('--tls', False)
    ca_cert = unquote_path(options.get('--tlscacert'))
    cert = unquote_path(options.get('--tlscert'))
    key = unquote_path(options.get('--tlskey'))
    verify = options.get('--tlsverify')
    skip_hostname_check = options.get('--skip-hostname-check', False)

    advanced_opts = any([ca_cert, cert, key, verify])

    if tls is True and not advanced_opts:
        return True
    elif advanced_opts:  # --tls is a noop
        client_cert = None
        if cert or key:
            client_cert = (cert, key)

        return TLSConfig(
            client_cert=client_cert, verify=verify, ca_cert=ca_cert,
            assert_hostname=False if skip_hostname_check else None
        )

    return None


def docker_client(environment, version=None, tls_config=None, host=None,
                  tls_version=None):
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    try:
        kwargs = kwargs_from_env(environment=environment, ssl_version=tls_version)
    except TLSParameterError:
        raise UserError(
            "TLS configuration is invalid - make sure your DOCKER_TLS_VERIFY "
            "and DOCKER_CERT_PATH are set correctly.\n"
            "You might need to run `eval \"$(docker-machine env default)\"`")

    if host:
        kwargs['base_url'] = host
    if tls_config:
        kwargs['tls'] = tls_config

    if version:
        kwargs['version'] = version

    timeout = environment.get('COMPOSE_HTTP_TIMEOUT')
    if timeout:
        kwargs['timeout'] = int(timeout)
    else:
        kwargs['timeout'] = HTTP_TIMEOUT

    kwargs['user_agent'] = generate_user_agent()

    return Client(**kwargs)
