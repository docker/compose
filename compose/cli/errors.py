from __future__ import absolute_import
from __future__ import unicode_literals

import contextlib
import logging
import socket
from distutils.spawn import find_executable
from textwrap import dedent

from docker.errors import APIError
from requests.exceptions import ConnectionError as RequestsConnectionError
from requests.exceptions import ReadTimeout
from requests.exceptions import SSLError
from requests.packages.urllib3.exceptions import ReadTimeoutError

from ..const import API_VERSION_TO_ENGINE_VERSION
from .utils import binarystr_to_unicode
from .utils import is_docker_for_mac_installed
from .utils import is_mac
from .utils import is_ubuntu
from .utils import is_windows


log = logging.getLogger(__name__)


class UserError(Exception):

    def __init__(self, msg):
        self.msg = dedent(msg).strip()

    def __unicode__(self):
        return self.msg

    __str__ = __unicode__


class ConnectionError(Exception):
    pass


@contextlib.contextmanager
def handle_connection_errors(client):
    try:
        yield
    except SSLError as e:
        log.error('SSL error: %s' % e)
        raise ConnectionError()
    except RequestsConnectionError as e:
        if e.args and isinstance(e.args[0], ReadTimeoutError):
            log_timeout_error(client.timeout)
            raise ConnectionError()
        exit_with_error(get_conn_error_message(client.base_url))
    except APIError as e:
        log_api_error(e, client.api_version)
        raise ConnectionError()
    except (ReadTimeout, socket.timeout) as e:
        log_timeout_error(client.timeout)
        raise ConnectionError()
    except Exception as e:
        if is_windows():
            import pywintypes
            if isinstance(e, pywintypes.error):
                log_windows_pipe_error(e)
                raise ConnectionError()
        raise


def log_windows_pipe_error(exc):
    if exc.winerror == 232:  # https://github.com/docker/compose/issues/5005
        log.error(
            "The current Compose file version is not compatible with your engine version. "
            "Please upgrade your Compose file to a more recent version, or set "
            "a COMPOSE_API_VERSION in your environment."
        )
    else:
        log.error(
            "Windows named pipe error: {} (code: {})".format(
                binarystr_to_unicode(exc.strerror), exc.winerror
            )
        )


def log_timeout_error(timeout):
    log.error(
        "An HTTP request took too long to complete. Retry with --verbose to "
        "obtain debug information.\n"
        "If you encounter this issue regularly because of slow network "
        "conditions, consider setting COMPOSE_HTTP_TIMEOUT to a higher "
        "value (current value: %s)." % timeout)


def log_api_error(e, client_version):
    explanation = binarystr_to_unicode(e.explanation)

    if 'client is newer than server' not in explanation:
        log.error(explanation)
        return

    version = API_VERSION_TO_ENGINE_VERSION.get(client_version)
    if not version:
        # They've set a custom API version
        log.error(explanation)
        return

    log.error(
        "The Docker Engine version is less than the minimum required by "
        "Compose. Your current project requires a Docker Engine of "
        "version {version} or greater.".format(version=version)
    )


def exit_with_error(msg):
    log.error(dedent(msg).strip())
    raise ConnectionError()


def get_conn_error_message(url):
    try:
        if find_executable('docker') is None:
            return docker_not_found_msg("Couldn't connect to Docker daemon.")
        if is_docker_for_mac_installed():
            return conn_error_docker_for_mac
        if find_executable('docker-machine') is not None:
            return conn_error_docker_machine
    except UnicodeDecodeError:
        # https://github.com/docker/compose/issues/5442
        # Ignore the error and print the generic message instead.
        pass
    return conn_error_generic.format(url=url)


def docker_not_found_msg(problem):
    return "{} You might need to install Docker:\n\n{}".format(
        problem, docker_install_url())


def docker_install_url():
    if is_mac():
        return docker_install_url_mac
    elif is_ubuntu():
        return docker_install_url_ubuntu
    elif is_windows():
        return docker_install_url_windows
    else:
        return docker_install_url_generic


docker_install_url_mac = "https://docs.docker.com/engine/installation/mac/"
docker_install_url_ubuntu = "https://docs.docker.com/engine/installation/ubuntulinux/"
docker_install_url_windows = "https://docs.docker.com/engine/installation/windows/"
docker_install_url_generic = "https://docs.docker.com/engine/installation/"


conn_error_docker_machine = """
    Couldn't connect to Docker daemon - you might need to run `docker-machine start default`.
"""

conn_error_docker_for_mac = """
    Couldn't connect to Docker daemon. You might need to start Docker for Mac.
"""


conn_error_generic = """
    Couldn't connect to Docker daemon at {url} - is it running?

    If it's at a non-standard location, specify the URL with the DOCKER_HOST environment variable.
"""
