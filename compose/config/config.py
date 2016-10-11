from __future__ import absolute_import
from __future__ import unicode_literals

import functools
import logging
import os
import string
import sys
from collections import namedtuple

import six
import yaml
from cached_property import cached_property

from ..const import COMPOSEFILE_V1 as V1
from ..const import COMPOSEFILE_V2_0 as V2_0
from ..const import COMPOSEFILE_V2_1 as V2_1
from ..utils import build_string_dict
from ..utils import splitdrive
from .environment import env_vars_from_file
from .environment import Environment
from .environment import split_env
from .errors import CircularReference
from .errors import ComposeFileNotFound
from .errors import ConfigurationError
from .errors import VERSION_EXPLANATION
from .interpolation import interpolate_environment_variables
from .sort_services import get_container_name_from_network_mode
from .sort_services import get_service_name_from_network_mode
from .sort_services import sort_service_dicts
from .types import parse_extra_hosts
from .types import parse_restart_spec
from .types import ServiceLink
from .types import VolumeFromSpec
from .types import VolumeSpec
from .validation import match_named_volumes
from .validation import validate_against_config_schema
from .validation import validate_config_section
from .validation import validate_depends_on
from .validation import validate_extends_file_path
from .validation import validate_links
from .validation import validate_network_mode
from .validation import validate_service_constraints
from .validation import validate_top_level_object
from .validation import validate_ulimits


DOCKER_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cgroup_parent',
    'command',
    'cpu_quota',
    'cpu_shares',
    'cpuset',
    'detach',
    'devices',
    'dns',
    'dns_search',
    'domainname',
    'entrypoint',
    'env_file',
    'environment',
    'extra_hosts',
    'group_add',
    'hostname',
    'image',
    'ipc',
    'labels',
    'links',
    'mac_address',
    'mem_limit',
    'memswap_limit',
    'mem_swappiness',
    'net',
    'oom_score_adj'
    'pid',
    'ports',
    'privileged',
    'read_only',
    'restart',
    'security_opt',
    'shm_size',
    'stdin_open',
    'stop_signal',
    'tty',
    'user',
    'volume_driver',
    'volumes',
    'volumes_from',
    'working_dir',
]

ALLOWED_KEYS = DOCKER_CONFIG_KEYS + [
    'build',
    'container_name',
    'dockerfile',
    'log_driver',
    'log_opt',
    'logging',
    'network_mode',
]

DOCKER_VALID_URL_PREFIXES = (
    'http://',
    'https://',
    'git://',
    'github.com/',
    'git@',
)

SUPPORTED_FILENAMES = [
    'docker-compose.yml',
    'docker-compose.yaml',
]

DEFAULT_OVERRIDE_FILENAME = 'docker-compose.override.yml'


log = logging.getLogger(__name__)


class ConfigDetails(namedtuple('_ConfigDetails', 'working_dir config_files environment')):
    """
    :param working_dir: the directory to use for relative paths in the config
    :type  working_dir: string
    :param config_files: list of configuration files to load
    :type  config_files: list of :class:`ConfigFile`
    :param environment: computed environment values for this project
    :type  environment: :class:`environment.Environment`
     """
    def __new__(cls, working_dir, config_files, environment=None):
        if environment is None:
            environment = Environment.from_env_file(working_dir)
        return super(ConfigDetails, cls).__new__(
            cls, working_dir, config_files, environment
        )


class ConfigFile(namedtuple('_ConfigFile', 'filename config')):
    """
    :param filename: filename of the config file
    :type  filename: string
    :param config: contents of the config file
    :type  config: :class:`dict`
    """

    @classmethod
    def from_filename(cls, filename):
        return cls(filename, load_yaml(filename))

    @cached_property
    def version(self):
        if 'version' not in self.config:
            return V1

        version = self.config['version']

        if isinstance(version, dict):
            log.warn('Unexpected type for "version" key in "{}". Assuming '
                     '"version" is the name of a service, and defaulting to '
                     'Compose file version 1.'.format(self.filename))
            return V1

        if not isinstance(version, six.string_types):
            raise ConfigurationError(
                'Version in "{}" is invalid - it should be a string.'
                .format(self.filename))

        if version == '1':
            raise ConfigurationError(
                'Version in "{}" is invalid. {}'
                .format(self.filename, VERSION_EXPLANATION))

        if version == '2':
            version = V2_0

        if version not in (V2_0, V2_1):
            raise ConfigurationError(
                'Version in "{}" is unsupported. {}'
                .format(self.filename, VERSION_EXPLANATION))

        return version

    def get_service(self, name):
        return self.get_service_dicts()[name]

    def get_service_dicts(self):
        return self.config if self.version == V1 else self.config.get('services', {})

    def get_volumes(self):
        return {} if self.version == V1 else self.config.get('volumes', {})

    def get_networks(self):
        return {} if self.version == V1 else self.config.get('networks', {})


class Config(namedtuple('_Config', 'version services volumes networks')):
    """
    :param version: configuration version
    :type  version: int
    :param services: List of service description dictionaries
    :type  services: :class:`list`
    :param volumes: Dictionary mapping volume names to description dictionaries
    :type  volumes: :class:`dict`
    :param networks: Dictionary mapping network names to description dictionaries
    :type  networks: :class:`dict`
    """


class ServiceConfig(namedtuple('_ServiceConfig', 'working_dir filename name config')):

    @classmethod
    def with_abs_paths(cls, working_dir, filename, name, config):
        if not working_dir:
            raise ValueError("No working_dir for ServiceConfig.")

        return cls(
            os.path.abspath(working_dir),
            os.path.abspath(filename) if filename else filename,
            name,
            config)


def find(base_dir, filenames, environment):
    if filenames == ['-']:
        return ConfigDetails(
            os.getcwd(),
            [ConfigFile(None, yaml.safe_load(sys.stdin))],
            environment
        )

    if filenames:
        filenames = [os.path.join(base_dir, f) for f in filenames]
    else:
        filenames = get_default_config_files(base_dir)

    log.debug("Using configuration files: {}".format(",".join(filenames)))
    return ConfigDetails(
        os.path.dirname(filenames[0]),
        [ConfigFile.from_filename(f) for f in filenames],
        environment
    )


def validate_config_version(config_files):
    main_file = config_files[0]
    validate_top_level_object(main_file)
    for next_file in config_files[1:]:
        validate_top_level_object(next_file)

        if main_file.version != next_file.version:
            raise ConfigurationError(
                "Version mismatch: file {0} specifies version {1} but "
                "extension file {2} uses version {3}".format(
                    main_file.filename,
                    main_file.version,
                    next_file.filename,
                    next_file.version))


def get_default_config_files(base_dir):
    (candidates, path) = find_candidates_in_parent_dirs(SUPPORTED_FILENAMES, base_dir)

    if not candidates:
        raise ComposeFileNotFound(SUPPORTED_FILENAMES)

    winner = candidates[0]

    if len(candidates) > 1:
        log.warn("Found multiple config files with supported names: %s", ", ".join(candidates))
        log.warn("Using %s\n", winner)

    return [os.path.join(path, winner)] + get_default_override_file(path)


def get_default_override_file(path):
    override_filename = os.path.join(path, DEFAULT_OVERRIDE_FILENAME)
    return [override_filename] if os.path.exists(override_filename) else []


def find_candidates_in_parent_dirs(filenames, path):
    """
    Given a directory path to start, looks for filenames in the
    directory, and then each parent directory successively,
    until found.

    Returns tuple (candidates, path).
    """
    candidates = [filename for filename in filenames
                  if os.path.exists(os.path.join(path, filename))]

    if not candidates:
        parent_dir = os.path.join(path, '..')
        if os.path.abspath(parent_dir) != os.path.abspath(path):
            return find_candidates_in_parent_dirs(filenames, parent_dir)

    return (candidates, path)


def load(config_details):
    """Load the configuration from a working directory and a list of
    configuration files.  Files are loaded in order, and merged on top
    of each other to create the final configuration.

    Return a fully interpolated, extended and validated configuration.
    """
    validate_config_version(config_details.config_files)

    processed_files = [
        process_config_file(config_file, config_details.environment)
        for config_file in config_details.config_files
    ]
    config_details = config_details._replace(config_files=processed_files)

    main_file = config_details.config_files[0]
    volumes = load_mapping(
        config_details.config_files, 'get_volumes', 'Volume'
    )
    networks = load_mapping(
        config_details.config_files, 'get_networks', 'Network'
    )
    service_dicts = load_services(config_details, main_file)

    if main_file.version != V1:
        for service_dict in service_dicts:
            match_named_volumes(service_dict, volumes)

    return Config(main_file.version, service_dicts, volumes, networks)


def load_mapping(config_files, get_func, entity_type):
    mapping = {}

    for config_file in config_files:
        for name, config in getattr(config_file, get_func)().items():
            mapping[name] = config or {}
            if not config:
                continue

            external = config.get('external')
            if external:
                if len(config.keys()) > 1:
                    raise ConfigurationError(
                        '{} {} declared as external but specifies'
                        ' additional attributes ({}). '.format(
                            entity_type,
                            name,
                            ', '.join([k for k in config.keys() if k != 'external'])
                        )
                    )
                if isinstance(external, dict):
                    config['external_name'] = external.get('name')
                else:
                    config['external_name'] = name

            mapping[name] = config

            if 'driver_opts' in config:
                config['driver_opts'] = build_string_dict(
                    config['driver_opts']
                )

            if 'labels' in config:
                config['labels'] = parse_labels(config['labels'])

    return mapping


def load_services(config_details, config_file):
    def build_service(service_name, service_dict, service_names):
        service_config = ServiceConfig.with_abs_paths(
            config_details.working_dir,
            config_file.filename,
            service_name,
            service_dict)
        resolver = ServiceExtendsResolver(
            service_config, config_file, environment=config_details.environment
        )
        service_dict = process_service(resolver.run())

        service_config = service_config._replace(config=service_dict)
        validate_service(service_config, service_names, config_file.version)
        service_dict = finalize_service(
            service_config,
            service_names,
            config_file.version,
            config_details.environment)
        return service_dict

    def build_services(service_config):
        service_names = service_config.keys()
        return sort_service_dicts([
            build_service(name, service_dict, service_names)
            for name, service_dict in service_config.items()
        ])

    def merge_services(base, override):
        all_service_names = set(base) | set(override)
        return {
            name: merge_service_dicts_from_files(
                base.get(name, {}),
                override.get(name, {}),
                config_file.version)
            for name in all_service_names
        }

    service_configs = [
        file.get_service_dicts() for file in config_details.config_files
    ]

    service_config = service_configs[0]
    for next_config in service_configs[1:]:
        service_config = merge_services(service_config, next_config)

    return build_services(service_config)


def interpolate_config_section(config_file, config, section, environment):
    validate_config_section(config_file.filename, config, section)
    return interpolate_environment_variables(
        config_file.version,
        config,
        section,
        environment
    )


def process_config_file(config_file, environment, service_name=None):
    services = interpolate_config_section(
        config_file,
        config_file.get_service_dicts(),
        'service',
        environment)

    if config_file.version in (V2_0, V2_1):
        processed_config = dict(config_file.config)
        processed_config['services'] = services
        processed_config['volumes'] = interpolate_config_section(
            config_file,
            config_file.get_volumes(),
            'volume',
            environment)
        processed_config['networks'] = interpolate_config_section(
            config_file,
            config_file.get_networks(),
            'network',
            environment)

    if config_file.version == V1:
        processed_config = services

    config_file = config_file._replace(config=processed_config)
    validate_against_config_schema(config_file)

    if service_name and service_name not in services:
        raise ConfigurationError(
            "Cannot extend service '{}' in {}: Service not found".format(
                service_name, config_file.filename))

    return config_file


class ServiceExtendsResolver(object):
    def __init__(self, service_config, config_file, environment, already_seen=None):
        self.service_config = service_config
        self.working_dir = service_config.working_dir
        self.already_seen = already_seen or []
        self.config_file = config_file
        self.environment = environment

    @property
    def signature(self):
        return self.service_config.filename, self.service_config.name

    def detect_cycle(self):
        if self.signature in self.already_seen:
            raise CircularReference(self.already_seen + [self.signature])

    def run(self):
        self.detect_cycle()

        if 'extends' in self.service_config.config:
            service_dict = self.resolve_extends(*self.validate_and_construct_extends())
            return self.service_config._replace(config=service_dict)

        return self.service_config

    def validate_and_construct_extends(self):
        extends = self.service_config.config['extends']
        if not isinstance(extends, dict):
            extends = {'service': extends}

        config_path = self.get_extended_config_path(extends)
        service_name = extends['service']

        extends_file = ConfigFile.from_filename(config_path)
        validate_config_version([self.config_file, extends_file])
        extended_file = process_config_file(
            extends_file, self.environment, service_name=service_name
        )
        service_config = extended_file.get_service(service_name)

        return config_path, service_config, service_name

    def resolve_extends(self, extended_config_path, service_dict, service_name):
        resolver = ServiceExtendsResolver(
            ServiceConfig.with_abs_paths(
                os.path.dirname(extended_config_path),
                extended_config_path,
                service_name,
                service_dict),
            self.config_file,
            already_seen=self.already_seen + [self.signature],
            environment=self.environment
        )

        service_config = resolver.run()
        other_service_dict = process_service(service_config)
        validate_extended_service_dict(
            other_service_dict,
            extended_config_path,
            service_name)

        return merge_service_dicts(
            other_service_dict,
            self.service_config.config,
            self.config_file.version)

    def get_extended_config_path(self, extends_options):
        """Service we are extending either has a value for 'file' set, which we
        need to obtain a full path too or we are extending from a service
        defined in our own file.
        """
        filename = self.service_config.filename
        validate_extends_file_path(
            self.service_config.name,
            extends_options,
            filename)
        if 'file' in extends_options:
            return expand_path(self.working_dir, extends_options['file'])
        return filename


def resolve_environment(service_dict, environment=None):
    """Unpack any environment variables from an env_file, if set.
    Interpolate environment values if set.
    """
    env = {}
    for env_file in service_dict.get('env_file', []):
        env.update(env_vars_from_file(env_file))

    env.update(parse_environment(service_dict.get('environment')))
    return dict(resolve_env_var(k, v, environment) for k, v in six.iteritems(env))


def resolve_build_args(build, environment):
    args = parse_build_arguments(build.get('args'))
    return dict(resolve_env_var(k, v, environment) for k, v in six.iteritems(args))


def validate_extended_service_dict(service_dict, filename, service):
    error_prefix = "Cannot extend service '%s' in %s:" % (service, filename)

    if 'links' in service_dict:
        raise ConfigurationError(
            "%s services with 'links' cannot be extended" % error_prefix)

    if 'volumes_from' in service_dict:
        raise ConfigurationError(
            "%s services with 'volumes_from' cannot be extended" % error_prefix)

    if 'net' in service_dict:
        if get_container_name_from_network_mode(service_dict['net']):
            raise ConfigurationError(
                "%s services with 'net: container' cannot be extended" % error_prefix)

    if 'network_mode' in service_dict:
        if get_service_name_from_network_mode(service_dict['network_mode']):
            raise ConfigurationError(
                "%s services with 'network_mode: service' cannot be extended" % error_prefix)

    if 'depends_on' in service_dict:
        raise ConfigurationError(
            "%s services with 'depends_on' cannot be extended" % error_prefix)


def validate_service(service_config, service_names, version):
    service_dict, service_name = service_config.config, service_config.name
    validate_service_constraints(service_dict, service_name, version)
    validate_paths(service_dict)

    validate_ulimits(service_config)
    validate_network_mode(service_config, service_names)
    validate_depends_on(service_config, service_names)
    validate_links(service_config, service_names)

    if not service_dict.get('image') and has_uppercase(service_name):
        raise ConfigurationError(
            "Service '{name}' contains uppercase characters which are not valid "
            "as part of an image name. Either use a lowercase service name or "
            "use the `image` field to set a custom name for the service image."
            .format(name=service_name))


def process_service(service_config):
    working_dir = service_config.working_dir
    service_dict = dict(service_config.config)

    if 'env_file' in service_dict:
        service_dict['env_file'] = [
            expand_path(working_dir, path)
            for path in to_list(service_dict['env_file'])
        ]

    if 'build' in service_dict:
        if isinstance(service_dict['build'], six.string_types):
            service_dict['build'] = resolve_build_path(working_dir, service_dict['build'])
        elif isinstance(service_dict['build'], dict) and 'context' in service_dict['build']:
            path = service_dict['build']['context']
            service_dict['build']['context'] = resolve_build_path(working_dir, path)

    if 'volumes' in service_dict and service_dict.get('volume_driver') is None:
        service_dict['volumes'] = resolve_volume_paths(working_dir, service_dict)

    if 'labels' in service_dict:
        service_dict['labels'] = parse_labels(service_dict['labels'])

    if 'extra_hosts' in service_dict:
        service_dict['extra_hosts'] = parse_extra_hosts(service_dict['extra_hosts'])

    for field in ['dns', 'dns_search', 'tmpfs']:
        if field in service_dict:
            service_dict[field] = to_list(service_dict[field])

    return service_dict


def finalize_service(service_config, service_names, version, environment):
    service_dict = dict(service_config.config)

    if 'environment' in service_dict or 'env_file' in service_dict:
        service_dict['environment'] = resolve_environment(service_dict, environment)
        service_dict.pop('env_file', None)

    if 'volumes_from' in service_dict:
        service_dict['volumes_from'] = [
            VolumeFromSpec.parse(vf, service_names, version)
            for vf in service_dict['volumes_from']
        ]

    if 'volumes' in service_dict:
        service_dict['volumes'] = [
            VolumeSpec.parse(
                v, environment.get('COMPOSE_CONVERT_WINDOWS_PATHS')
            ) for v in service_dict['volumes']
        ]

    if 'net' in service_dict:
        network_mode = service_dict.pop('net')
        container_name = get_container_name_from_network_mode(network_mode)
        if container_name and container_name in service_names:
            service_dict['network_mode'] = 'service:{}'.format(container_name)
        else:
            service_dict['network_mode'] = network_mode

    if 'networks' in service_dict:
        service_dict['networks'] = parse_networks(service_dict['networks'])

    if 'restart' in service_dict:
        service_dict['restart'] = parse_restart_spec(service_dict['restart'])

    normalize_build(service_dict, service_config.working_dir, environment)

    service_dict['name'] = service_config.name
    return normalize_v1_service_format(service_dict)


def normalize_v1_service_format(service_dict):
    if 'log_driver' in service_dict or 'log_opt' in service_dict:
        if 'logging' not in service_dict:
            service_dict['logging'] = {}
        if 'log_driver' in service_dict:
            service_dict['logging']['driver'] = service_dict['log_driver']
            del service_dict['log_driver']
        if 'log_opt' in service_dict:
            service_dict['logging']['options'] = service_dict['log_opt']
            del service_dict['log_opt']

    if 'dockerfile' in service_dict:
        service_dict['build'] = service_dict.get('build', {})
        service_dict['build'].update({
            'dockerfile': service_dict.pop('dockerfile')
        })

    return service_dict


def merge_service_dicts_from_files(base, override, version):
    """When merging services from multiple files we need to merge the `extends`
    field. This is not handled by `merge_service_dicts()` which is used to
    perform the `extends`.
    """
    new_service = merge_service_dicts(base, override, version)
    if 'extends' in override:
        new_service['extends'] = override['extends']
    elif 'extends' in base:
        new_service['extends'] = base['extends']
    return new_service


class MergeDict(dict):
    """A dict-like object responsible for merging two dicts into one."""

    def __init__(self, base, override):
        self.base = base
        self.override = override

    def needs_merge(self, field):
        return field in self.base or field in self.override

    def merge_field(self, field, merge_func, default=None):
        if not self.needs_merge(field):
            return

        self[field] = merge_func(
            self.base.get(field, default),
            self.override.get(field, default))

    def merge_mapping(self, field, parse_func):
        if not self.needs_merge(field):
            return

        self[field] = parse_func(self.base.get(field))
        self[field].update(parse_func(self.override.get(field)))

    def merge_sequence(self, field, parse_func):
        def parse_sequence_func(seq):
            return to_mapping((parse_func(item) for item in seq), 'merge_field')

        if not self.needs_merge(field):
            return

        merged = parse_sequence_func(self.base.get(field, []))
        merged.update(parse_sequence_func(self.override.get(field, [])))
        self[field] = [item.repr() for item in sorted(merged.values())]

    def merge_scalar(self, field):
        if self.needs_merge(field):
            self[field] = self.override.get(field, self.base.get(field))


def merge_service_dicts(base, override, version):
    md = MergeDict(base, override)

    md.merge_mapping('environment', parse_environment)
    md.merge_mapping('labels', parse_labels)
    md.merge_mapping('ulimits', parse_ulimits)
    md.merge_mapping('networks', parse_networks)
    md.merge_sequence('links', ServiceLink.parse)

    for field in ['volumes', 'devices']:
        md.merge_field(field, merge_path_mappings)

    for field in [
        'ports', 'cap_add', 'cap_drop', 'expose', 'external_links',
        'security_opt', 'volumes_from', 'depends_on',
    ]:
        md.merge_field(field, merge_unique_items_lists, default=[])

    for field in ['dns', 'dns_search', 'env_file', 'tmpfs']:
        md.merge_field(field, merge_list_or_string)

    md.merge_field('logging', merge_logging)

    for field in set(ALLOWED_KEYS) - set(md):
        md.merge_scalar(field)

    if version == V1:
        legacy_v1_merge_image_or_build(md, base, override)
    elif md.needs_merge('build'):
        md['build'] = merge_build(md, base, override)

    return dict(md)


def merge_unique_items_lists(base, override):
    return sorted(set().union(base, override))


def merge_build(output, base, override):
    def to_dict(service):
        build_config = service.get('build', {})
        if isinstance(build_config, six.string_types):
            return {'context': build_config}
        return build_config

    md = MergeDict(to_dict(base), to_dict(override))
    md.merge_scalar('context')
    md.merge_scalar('dockerfile')
    md.merge_mapping('args', parse_build_arguments)
    return dict(md)


def merge_logging(base, override):
    md = MergeDict(base, override)
    md.merge_scalar('driver')
    if md.get('driver') == base.get('driver') or base.get('driver') is None:
        md.merge_mapping('options', lambda m: m or {})
    else:
        md['options'] = override.get('options')
    return dict(md)


def legacy_v1_merge_image_or_build(output, base, override):
    output.pop('image', None)
    output.pop('build', None)
    if 'image' in override:
        output['image'] = override['image']
    elif 'build' in override:
        output['build'] = override['build']
    elif 'image' in base:
        output['image'] = base['image']
    elif 'build' in base:
        output['build'] = base['build']


def merge_environment(base, override):
    env = parse_environment(base)
    env.update(parse_environment(override))
    return env


def split_label(label):
    if '=' in label:
        return label.split('=', 1)
    else:
        return label, ''


def parse_dict_or_list(split_func, type_name, arguments):
    if not arguments:
        return {}

    if isinstance(arguments, list):
        return dict(split_func(e) for e in arguments)

    if isinstance(arguments, dict):
        return dict(arguments)

    raise ConfigurationError(
        "%s \"%s\" must be a list or mapping," %
        (type_name, arguments)
    )


parse_build_arguments = functools.partial(parse_dict_or_list, split_env, 'build arguments')
parse_environment = functools.partial(parse_dict_or_list, split_env, 'environment')
parse_labels = functools.partial(parse_dict_or_list, split_label, 'labels')
parse_networks = functools.partial(parse_dict_or_list, lambda k: (k, None), 'networks')


def parse_ulimits(ulimits):
    if not ulimits:
        return {}

    if isinstance(ulimits, dict):
        return dict(ulimits)


def resolve_env_var(key, val, environment):
    if val is not None:
        return key, val
    elif environment and key in environment:
        return key, environment[key]
    else:
        return key, None


def resolve_volume_paths(working_dir, service_dict):
    return [
        resolve_volume_path(working_dir, volume)
        for volume in service_dict['volumes']
    ]


def resolve_volume_path(working_dir, volume):
    container_path, host_path = split_path_mapping(volume)

    if host_path is not None:
        if host_path.startswith('.'):
            host_path = expand_path(working_dir, host_path)
        host_path = os.path.expanduser(host_path)
        return u"{}:{}".format(host_path, container_path)
    else:
        return container_path


def normalize_build(service_dict, working_dir, environment):

    if 'build' in service_dict:
        build = {}
        # Shortcut where specifying a string is treated as the build context
        if isinstance(service_dict['build'], six.string_types):
            build['context'] = service_dict.pop('build')
        else:
            build.update(service_dict['build'])
            if 'args' in build:
                build['args'] = build_string_dict(
                    resolve_build_args(build, environment)
                )

        service_dict['build'] = build


def resolve_build_path(working_dir, build_path):
    if is_url(build_path):
        return build_path
    return expand_path(working_dir, build_path)


def is_url(build_path):
    return build_path.startswith(DOCKER_VALID_URL_PREFIXES)


def validate_paths(service_dict):
    if 'build' in service_dict:
        build = service_dict.get('build', {})

        if isinstance(build, six.string_types):
            build_path = build
        elif isinstance(build, dict) and 'context' in build:
            build_path = build['context']
        else:
            # We have a build section but no context, so nothing to validate
            return

        if (
            not is_url(build_path) and
            (not os.path.exists(build_path) or not os.access(build_path, os.R_OK))
        ):
            raise ConfigurationError(
                "build path %s either does not exist, is not accessible, "
                "or is not a valid URL." % build_path)


def merge_path_mappings(base, override):
    d = dict_from_path_mappings(base)
    d.update(dict_from_path_mappings(override))
    return path_mappings_from_dict(d)


def dict_from_path_mappings(path_mappings):
    if path_mappings:
        return dict(split_path_mapping(v) for v in path_mappings)
    else:
        return {}


def path_mappings_from_dict(d):
    return [join_path_mapping(v) for v in sorted(d.items())]


def split_path_mapping(volume_path):
    """
    Ascertain if the volume_path contains a host path as well as a container
    path. Using splitdrive so windows absolute paths won't cause issues with
    splitting on ':'.
    """
    drive, volume_config = splitdrive(volume_path)

    if ':' in volume_config:
        (host, container) = volume_config.split(':', 1)
        return (container, drive + host)
    else:
        return (volume_path, None)


def join_path_mapping(pair):
    (container, host) = pair
    if host is None:
        return container
    else:
        return ":".join((host, container))


def expand_path(working_dir, path):
    return os.path.abspath(os.path.join(working_dir, os.path.expanduser(path)))


def merge_list_or_string(base, override):
    return to_list(base) + to_list(override)


def to_list(value):
    if value is None:
        return []
    elif isinstance(value, six.string_types):
        return [value]
    else:
        return value


def to_mapping(sequence, key_field):
    return {getattr(item, key_field): item for item in sequence}


def has_uppercase(name):
    return any(char in string.ascii_uppercase for char in name)


def load_yaml(filename):
    try:
        with open(filename, 'r') as fh:
            return yaml.safe_load(fh)
    except (IOError, yaml.YAMLError) as e:
        error_name = getattr(e, '__module__', '') + '.' + e.__class__.__name__
        raise ConfigurationError(u"{}: {}".format(error_name, e))
