from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os
from collections import namedtuple

from docker import Client
from docker.errors import TLSParameterError
from docker.tls import TLSConfig
from docker.utils import kwargs_from_env

from ..const import HTTP_TIMEOUT
from .errors import UserError

log = logging.getLogger(__name__)


class TLSArgs(namedtuple('_TLSArgs', 'tls cert key ca_cert verify')):
    @classmethod
    def from_options(cls, options):
        return cls(
            tls=options.get('--tls', False),
            ca_cert=options.get('--tlscacert'),
            cert=options.get('--tlscert'),
            key=options.get('--tlskey'),
            verify=options.get('--tlsverify')
        )

    # def has_config(self):
    #     return (
    #         self.tls or self.ca_cert or self.cert or self.key or self.verify
    #     )


def docker_client(version=None, tls_args=None, host=None):
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    if 'DOCKER_CLIENT_TIMEOUT' in os.environ:
        log.warn("The DOCKER_CLIENT_TIMEOUT environment variable is deprecated.  "
                 "Please use COMPOSE_HTTP_TIMEOUT instead.")

    try:
        kwargs = kwargs_from_env(assert_hostname=False)
    except TLSParameterError:
        raise UserError(
            "TLS configuration is invalid - make sure your DOCKER_TLS_VERIFY "
            "and DOCKER_CERT_PATH are set correctly.\n"
            "You might need to run `eval \"$(docker-machine env default)\"`")

    if host:
        kwargs['base_url'] = host
    if tls_args and any(tls_args):
        if tls_args.tls is True:
            kwargs['tls'] = True
        else:
            client_cert = None
            if tls_args.cert or tls_args.key:
                client_cert = (tls_args.cert, tls_args.key)
            try:
                kwargs['tls'] = TLSConfig(
                    client_cert=client_cert, verify=tls_args.verify,
                    ca_cert=tls_args.ca_cert
                )
            except TLSParameterError as e:
                raise UserError(
                    "TLS configuration is invalid. Please double-check the "
                    "TLS command-line arguments. ({0})".format(e)
                )

    if version:
        kwargs['version'] = version

    kwargs['timeout'] = HTTP_TIMEOUT

    return Client(**kwargs)
