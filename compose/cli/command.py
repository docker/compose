import logging
import os
import re

from . import errors
from .. import config
from .. import parallel
from ..config.environment import Environment
from ..const import LABEL_CONFIG_FILES
from ..const import LABEL_ENVIRONMENT_FILE
from ..const import LABEL_WORKING_DIR
from ..project import Project
from .docker_client import get_client
from .docker_client import load_context
from .docker_client import make_context
from .errors import UserError

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
    override_dir = get_project_dir(options)
    environment_file = options.get('--env-file')
    environment = Environment.from_env_file(override_dir or project_dir, environment_file)
    environment.silent = options.get('COMMAND', None) in SILENT_COMMANDS
    set_parallel_limit(environment)

    # get the context for the run
    context = None
    context_name = options.get('--context', None)
    if context_name:
        context = load_context(context_name)
        if not context:
            raise UserError("Context '{}' not found".format(context_name))

    host = options.get('--host', None)
    if host is not None:
        if context:
            raise UserError(
                "-H, --host and -c, --context are mutually exclusive. Only one should be set.")
        host = host.lstrip('=')
        context = make_context(host, options, environment)

    return get_project(
        project_dir,
        get_config_path_from_options(options, environment),
        project_name=options.get('--project-name'),
        verbose=options.get('--verbose'),
        context=context,
        environment=environment,
        override_dir=override_dir,
        interpolate=(not additional_options.get('--no-interpolate')),
        environment_file=environment_file,
        enabled_profiles=get_profiles_from_options(options, environment)
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


def get_project_dir(options):
    override_dir = None
    files = get_config_path_from_options(options, os.environ)
    if files:
        if files[0] == '-':
            return '.'
        override_dir = os.path.dirname(files[0])
    return options.get('--project-directory') or override_dir


def get_config_from_options(base_dir, options, additional_options=None):
    additional_options = additional_options or {}
    override_dir = get_project_dir(options)
    environment_file = options.get('--env-file')
    environment = Environment.from_env_file(override_dir or base_dir, environment_file)
    config_path = get_config_path_from_options(options, environment)
    return config.load(
        config.find(base_dir, config_path, environment, override_dir),
        not additional_options.get('--no-interpolate')
    )


def get_config_path_from_options(options, environment):
    def unicode_paths(paths):
        return [p.decode('utf-8') if isinstance(p, bytes) else p for p in paths]

    file_option = options.get('--file')
    if file_option:
        return unicode_paths(file_option)

    config_files = environment.get('COMPOSE_FILE')
    if config_files:
        pathsep = environment.get('COMPOSE_PATH_SEPARATOR', os.pathsep)
        return unicode_paths(config_files.split(pathsep))
    return None


def get_profiles_from_options(options, environment):
    profile_option = options.get('--profile')
    if profile_option:
        return profile_option

    profiles = environment.get('COMPOSE_PROFILES')
    if profiles:
        return profiles.split(',')

    return []


def get_project(project_dir, config_path=None, project_name=None, verbose=False,
                context=None, environment=None, override_dir=None,
                interpolate=True, environment_file=None, enabled_profiles=None):
    if not environment:
        environment = Environment.from_env_file(project_dir)
    config_details = config.find(project_dir, config_path, environment, override_dir)
    project_name = get_project_name(
        config_details.working_dir, project_name, environment
    )
    config_data = config.load(config_details, interpolate)

    api_version = environment.get('COMPOSE_API_VERSION')

    client = get_client(
        verbose=verbose, version=api_version, context=context, environment=environment
    )

    with errors.handle_connection_errors(client):
        return Project.from_config(
            project_name,
            config_data,
            client,
            environment.get('DOCKER_DEFAULT_PLATFORM'),
            execution_context_labels(config_details, environment_file),
            enabled_profiles,
        )


def execution_context_labels(config_details, environment_file):
    extra_labels = [
        '{}={}'.format(LABEL_WORKING_DIR, os.path.abspath(config_details.working_dir))
    ]

    if not use_config_from_stdin(config_details):
        extra_labels.append('{}={}'.format(LABEL_CONFIG_FILES, config_files_label(config_details)))

    if environment_file is not None:
        extra_labels.append('{}={}'.format(
            LABEL_ENVIRONMENT_FILE,
            os.path.normpath(environment_file))
            )
    return extra_labels


def use_config_from_stdin(config_details):
    for c in config_details.config_files:
        if not c.filename:
            return True
    return False


def config_files_label(config_details):
    return ",".join(
        os.path.normpath(c.filename) for c in config_details.config_files
        )


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
