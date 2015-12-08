from __future__ import absolute_import
from __future__ import unicode_literals

import contextlib
import logging
import os
import re

import six
from requests.exceptions import ConnectionError
from requests.exceptions import SSLError

from . import errors
from . import verbose_proxy
from .. import config
from ..project import Project
from .docker_client import docker_client
from .utils import call_silently
from .utils import get_version_info
from .utils import is_mac
from .utils import is_ubuntu

log = logging.getLogger(__name__)


@contextlib.contextmanager
def friendly_error_message():
    try:
        yield
    except SSLError as e:
        raise errors.UserError('SSL error: %s' % e)
    except ConnectionError:
        if call_silently(['which', 'docker']) != 0:
            if is_mac():
                raise errors.DockerNotFoundMac()
            elif is_ubuntu():
                raise errors.DockerNotFoundUbuntu()
            else:
                raise errors.DockerNotFoundGeneric()
        elif call_silently(['which', 'docker-machine']) == 0:
            raise errors.ConnectionErrorDockerMachine()
        else:
            raise errors.ConnectionErrorGeneric(get_client().base_url)


def project_from_options(base_dir, options):
    return get_project(
        base_dir,
        get_config_path_from_options(options),
        project_name=options.get('--project-name'),
        verbose=options.get('--verbose'),
        use_networking=options.get('--x-networking'),
        network_driver=options.get('--x-network-driver'),
    )


def get_config_path_from_options(options):
    file_option = options.get('--file')
    if file_option:
        return file_option

    if 'FIG_FILE' in os.environ:
        log.warn('The FIG_FILE environment variable is deprecated.')
        log.warn('Please use COMPOSE_FILE instead.')

    config_file = os.environ.get('COMPOSE_FILE') or os.environ.get('FIG_FILE')
    return [config_file] if config_file else None


def get_client(verbose=False, version=None):
    client = docker_client(version=version)
    if verbose:
        version_info = six.iteritems(client.version())
        log.info(get_version_info('full'))
        log.info("Docker base_url: %s", client.base_url)
        log.info("Docker version: %s",
                 ", ".join("%s=%s" % item for item in version_info))
        return verbose_proxy.VerboseProxy('docker', client)
    return client


def get_project(base_dir, config_path=None, project_name=None, verbose=False,
                use_networking=False, network_driver=None):
    config_details = config.find(base_dir, config_path)

    api_version = '1.21' if use_networking else None
    return Project.from_dicts(
        get_project_name(config_details.working_dir, project_name),
        config.load(config_details),
        get_client(verbose=verbose, version=api_version),
        use_networking=use_networking,
        network_driver=network_driver)


def get_project_name(working_dir, project_name=None):
    def normalize_name(name):
        return re.sub(r'[^a-z0-9]', '', name.lower())

    if 'FIG_PROJECT_NAME' in os.environ:
        log.warn('The FIG_PROJECT_NAME environment variable is deprecated.')
        log.warn('Please use COMPOSE_PROJECT_NAME instead.')

    project_name = (
        project_name or
        os.environ.get('COMPOSE_PROJECT_NAME') or
        os.environ.get('FIG_PROJECT_NAME'))
    if project_name is not None:
        return normalize_name(project_name)

    project = os.path.basename(os.path.abspath(working_dir))
    if project:
        return normalize_name(project)

    return 'default'
