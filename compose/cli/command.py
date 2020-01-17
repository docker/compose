from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os
import re

import six

from . import errors
from . import verbose_proxy
from .. import config
from .. import parallel
from ..config.environment import Environment
from ..const import API_VERSIONS
from ..const import LABEL_CONFIG_FILES
from ..const import LABEL_ENVIRONMENT_FILE
from ..const import LABEL_WORKING_DIR
from ..project import Project
from .docker_client import docker_client
from .docker_client import get_tls_version
from .docker_client import tls_config_from_options
from .utils import get_version_info

log = logging.getLogger(__name__)

SILENT_COMMANDS = {
    'events',
    'exec',
    'kill',
    'logs',
    'pause',
    'ps',
    'restart',
    'rm',
    'start',
    'stop',
    'top',
    'unpause',
}


def project_from_options(project_dir, options, additional_options=None):
    additional_options = additional_options or {}
    override_dir = options.get('--project-directory')
    environment_file = options.get('--env-file')
    environment = Environment.from_env_file(override_dir or project_dir, environment_file)
    environment.silent = options.get('COMMAND', None) in SILENT_COMMANDS
    set_parallel_limit(environment)

    host = options.get('--host')
    if host is not None:
        host = host.lstrip('=')
    return get_project(
        project_dir,
        get_config_path_from_options(project_dir, options, environment),
        project_name=options.get('--project-name'),
        verbose=options.get('--verbose'),
        host=host,
        tls_config=tls_config_from_options(options, environment),
        environment=environment,
        override_dir=override_dir,
        compatibility=compatibility_from_options(project_dir, options, environment),
        interpolate=(not additional_options.get('--no-interpolate')),
        environment_file=environment_file
    )


def set_parallel_limit(environment):
    parallel_limit = environment.get('COMPOSE_PARALLEL_LIMIT')
    if parallel_limit:
        try:
            parallel_limit = int(parallel_limit)
        except ValueError:
            raise errors.UserError(
                'COMPOSE_PARALLEL_LIMIT must be an integer (found: "{}")'.format(
                    environment.get('COMPOSE_PARALLEL_LIMIT')
                )
            )
        if parallel_limit <= 1:
            raise errors.UserError('COMPOSE_PARALLEL_LIMIT can not be less than 2')
        parallel.GlobalLimit.set_global_limit(parallel_limit)


def get_config_from_options(base_dir, options, additional_options=None):
    additional_options = additional_options or {}
    override_dir = options.get('--project-directory')
    environment_file = options.get('--env-file')
    environment = Environment.from_env_file(override_dir or base_dir, environment_file)
    config_path = get_config_path_from_options(
        base_dir, options, environment
    )
    return config.load(
        config.find(base_dir, config_path, environment, override_dir),
        compatibility_from_options(config_path, options, environment),
        not additional_options.get('--no-interpolate')
    )


def get_config_path_from_options(base_dir, options, environment):
    def unicode_paths(paths):
        return [p.decode('utf-8') if isinstance(p, six.binary_type) else p for p in paths]

    file_option = options.get('--file')
    if file_option:
        return unicode_paths(file_option)

    config_files = environment.get('COMPOSE_FILE')
    if config_files:
        pathsep = environment.get('COMPOSE_PATH_SEPARATOR', os.pathsep)
        return unicode_paths(config_files.split(pathsep))
    return None


def get_client(environment, verbose=False, version=None, tls_config=None, host=None,
               tls_version=None):

    client = docker_client(
        version=version, tls_config=tls_config, host=host,
        environment=environment, tls_version=get_tls_version(environment)
    )
    if verbose:
        version_info = six.iteritems(client.version())
        log.info(get_version_info('full'))
        log.info("Docker base_url: %s", client.base_url)
        log.info("Docker version: %s",
                 ", ".join("%s=%s" % item for item in version_info))
        return verbose_proxy.VerboseProxy('docker', client)
    return client


def get_project(project_dir, config_path=None, project_name=None, verbose=False,
                host=None, tls_config=None, environment=None, override_dir=None,
                compatibility=False, interpolate=True, environment_file=None):
    if not environment:
        environment = Environment.from_env_file(project_dir)
    config_details = config.find(project_dir, config_path, environment, override_dir)
    project_name = get_project_name(
        config_details.working_dir, project_name, environment
    )
    config_data = config.load(config_details, compatibility, interpolate)

    api_version = environment.get(
        'COMPOSE_API_VERSION',
        API_VERSIONS[config_data.version])

    client = get_client(
        verbose=verbose, version=api_version, tls_config=tls_config,
        host=host, environment=environment
    )

    with errors.handle_connection_errors(client):
        return Project.from_config(
            project_name,
            config_data,
            client,
            environment.get('DOCKER_DEFAULT_PLATFORM'),
            execution_context_labels(config_details, environment_file),
        )


def execution_context_labels(config_details, environment_file):
    extra_labels = [
        '{0}={1}'.format(LABEL_WORKING_DIR, os.path.abspath(config_details.working_dir))
    ]

    if not use_config_from_stdin(config_details):
        extra_labels.append('{0}={1}'.format(LABEL_CONFIG_FILES, config_files_label(config_details)))

    if environment_file is not None:
        extra_labels.append('{0}={1}'.format(LABEL_ENVIRONMENT_FILE,
                                             os.path.normpath(environment_file)))
    return extra_labels


def use_config_from_stdin(config_details):
    for c in config_details.config_files:
        if not c.filename:
            return True
    return False


def config_files_label(config_details):
    return ",".join(
        map(str, (os.path.normpath(c.filename) for c in config_details.config_files)))


def get_project_name(working_dir, project_name=None, environment=None):
    def normalize_name(name):
        return re.sub(r'[^-_a-z0-9]', '', name.lower())

    if not environment:
        environment = Environment.from_env_file(working_dir)
    project_name = project_name or environment.get('COMPOSE_PROJECT_NAME')
    if project_name:
        return normalize_name(project_name)

    project = os.path.basename(os.path.abspath(working_dir))
    if project:
        return normalize_name(project)

    return 'default'


def compatibility_from_options(working_dir, options=None, environment=None):
    """Get compose v3 compatibility from --compatibility option
       or from COMPOSE_COMPATIBILITY environment variable."""

    compatibility_option = options.get('--compatibility')
    compatibility_environment = environment.get_boolean('COMPOSE_COMPATIBILITY')

    return compatibility_option or compatibility_environment
