import logging
import os.path
import ssl

from docker import APIClient
from docker import Context
from docker import ContextAPI
from docker import TLSConfig
from docker.errors import TLSParameterError
from docker.utils import kwargs_from_env
from docker.utils.config import home_dir

from . import verbose_proxy
from ..config.environment import Environment
from ..const import HTTP_TIMEOUT
from ..utils import unquote_path
from .errors import UserError
from .utils import generate_user_agent
from .utils import get_version_info

log = logging.getLogger(__name__)


def default_cert_path():
    return os.path.join(home_dir(), '.docker')


def make_context(host, options, environment):
    tls = tls_config_from_options(options, environment)
    ctx = Context("compose", host=host, tls=tls.verify if tls else False)
    if tls:
        ctx.set_endpoint("docker", host, tls, skip_tls_verify=not tls.verify)
    return ctx


def load_context(name=None):
    return ContextAPI.get_context(name)


def get_client(environment, verbose=False, version=None, context=None):
    client = docker_client(
        version=version, context=context,
        environment=environment, tls_version=get_tls_version(environment)
    )
    if verbose:
        version_info = client.version().items()
        log.info(get_version_info('full'))
        log.info("Docker base_url: %s", client.base_url)
        log.info("Docker version: %s",
                 ", ".join("%s=%s" % item for item in version_info))
        return verbose_proxy.VerboseProxy('docker', client)
    return client


def get_tls_version(environment):
    compose_tls_version = environment.get('COMPOSE_TLS_VERSION', None)
    if not compose_tls_version:
        return None

    tls_attr_name = "PROTOCOL_{}".format(compose_tls_version)
    if not hasattr(ssl, tls_attr_name):
        log.warning(
            'The "{}" protocol is unavailable. You may need to update your '
            'version of Python or OpenSSL. Falling back to TLSv1 (default).'
            .format(compose_tls_version)
        )
        return None

    return getattr(ssl, tls_attr_name)


def tls_config_from_options(options, environment=None):
    environment = environment or Environment()
    cert_path = environment.get('DOCKER_CERT_PATH') or None

    tls = options.get('--tls', False)
    ca_cert = unquote_path(options.get('--tlscacert'))
    cert = unquote_path(options.get('--tlscert'))
    key = unquote_path(options.get('--tlskey'))
    # verify is a special case - with docopt `--tlsverify` = False means it
    # wasn't used, so we set it if either the environment or the flag is True
    # see https://github.com/docker/compose/issues/5632
    verify = options.get('--tlsverify') or environment.get_boolean('DOCKER_TLS_VERIFY')

    skip_hostname_check = options.get('--skip-hostname-check', False)
    if cert_path is not None and not any((ca_cert, cert, key)):
        # FIXME: Modify TLSConfig to take a cert_path argument and do this internally
        cert = os.path.join(cert_path, 'cert.pem')
        key = os.path.join(cert_path, 'key.pem')
        ca_cert = os.path.join(cert_path, 'ca.pem')

    if verify and not any((ca_cert, cert, key)):
        # Default location for cert files is ~/.docker
        ca_cert = os.path.join(default_cert_path(), 'ca.pem')
        cert = os.path.join(default_cert_path(), 'cert.pem')
        key = os.path.join(default_cert_path(), 'key.pem')

    tls_version = get_tls_version(environment)

    advanced_opts = any([ca_cert, cert, key, verify, tls_version])

    if tls is True and not advanced_opts:
        return True
    elif advanced_opts:  # --tls is a noop
        client_cert = None
        if cert or key:
            client_cert = (cert, key)

        return TLSConfig(
            client_cert=client_cert, verify=verify, ca_cert=ca_cert,
            assert_hostname=False if skip_hostname_check else None,
            ssl_version=tls_version
        )

    return None


def docker_client(environment, version=None, context=None, tls_version=None):
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

    if not context:
        # check env for DOCKER_HOST and certs path
        host = kwargs.get("base_url", None)
        tls = kwargs.get("tls", None)
        verify = False if not tls else tls.verify
        if host:
            context = Context("compose", host=host, tls=verify)
        else:
            context = ContextAPI.get_current_context()
        if tls:
            context.set_endpoint("docker", host=host, tls_cfg=tls, skip_tls_verify=not verify)

    if not context.is_docker_host():
        raise UserError(
            "The platform targeted with the current context is not supported.\n"
            "Make sure the context in use targets a Docker Engine.\n")

    kwargs['base_url'] = context.Host
    if context.TLSConfig:
        kwargs['tls'] = context.TLSConfig

    if version:
        kwargs['version'] = version

    timeout = environment.get('COMPOSE_HTTP_TIMEOUT')
    if timeout:
        kwargs['timeout'] = int(timeout)
    else:
        kwargs['timeout'] = HTTP_TIMEOUT

    kwargs['user_agent'] = generate_user_agent()

    # Workaround for
    # https://pyinstaller.readthedocs.io/en/v3.3.1/runtime-information.html#ld-library-path-libpath-considerations
    if 'LD_LIBRARY_PATH_ORIG' in environment:
        kwargs['credstore_env'] = {
            'LD_LIBRARY_PATH': environment.get('LD_LIBRARY_PATH_ORIG'),
        }
    use_paramiko_ssh = int(environment.get('COMPOSE_PARAMIKO_SSH', 0))
    client = APIClient(use_ssh_client=not use_paramiko_ssh, **kwargs)
    client._original_base_url = kwargs.get('base_url')

    return client
