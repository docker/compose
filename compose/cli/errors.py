from __future__ import absolute_import
from __future__ import unicode_literals

import contextlib
import logging
import socket
from textwrap import dedent

from docker.errors import APIError
from requests.exceptions import ConnectionError as RequestsConnectionError
from requests.exceptions import ReadTimeout
from requests.exceptions import SSLError
from requests.packages.urllib3.exceptions import ReadTimeoutError

from ..const import API_VERSION_TO_ENGINE_VERSION
from ..const import HTTP_TIMEOUT
from .utils import call_silently
from .utils import is_mac
from .utils import is_ubuntu


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
            log_timeout_error()
            raise ConnectionError()

        if call_silently(['which', 'docker']) != 0:
            if is_mac():
                exit_with_error(docker_not_found_mac)
            if is_ubuntu():
                exit_with_error(docker_not_found_ubuntu)
            exit_with_error(docker_not_found_generic)
        if call_silently(['which', 'docker-machine']) == 0:
            exit_with_error(conn_error_docker_machine)
        exit_with_error(conn_error_generic.format(url=client.base_url))
    except APIError as e:
        log_api_error(e, client.api_version)
        raise ConnectionError()
    except (ReadTimeout, socket.timeout) as e:
        log_timeout_error()
        raise ConnectionError()


def log_timeout_error():
    log.error(
        "An HTTP request took too long to complete. Retry with --verbose to "
        "obtain debug information.\n"
        "If you encounter this issue regularly because of slow network "
        "conditions, consider setting COMPOSE_HTTP_TIMEOUT to a higher "
        "value (current value: %s)." % HTTP_TIMEOUT)


def log_api_error(e, client_version):
    if b'client is newer than server' not in e.explanation:
        log.error(e.explanation)
        return

    version = API_VERSION_TO_ENGINE_VERSION.get(client_version)
    if not version:
        # They've set a custom API version
        log.error(e.explanation)
        return

    log.error(
        "The Docker Engine version is less than the minimum required by "
        "Compose. Your current project requires a Docker Engine of "
        "version {version} or greater.".format(version=version))


def exit_with_error(msg):
    log.error(dedent(msg).strip())
    raise ConnectionError()


docker_not_found_mac = """
    Couldn't connect to Docker daemon. You might need to install Docker:

    https://docs.docker.com/engine/installation/mac/
"""


docker_not_found_ubuntu = """
    Couldn't connect to Docker daemon. You might need to install Docker:

    https://docs.docker.com/engine/installation/ubuntulinux/
"""


docker_not_found_generic = """
    Couldn't connect to Docker daemon. You might need to install Docker:

    https://docs.docker.com/engine/installation/
"""


conn_error_docker_machine = """
    Couldn't connect to Docker daemon - you might need to run `docker-machine start default`.
"""


conn_error_generic = """
    Couldn't connect to Docker daemon at {url} - is it running?

    If it's at a non-standard location, specify the URL with the DOCKER_HOST environment variable.
"""
