import functools
import logging
import os
import re
import string
import sys
from collections import namedtuple
from itertools import chain
from operator import attrgetter
from operator import itemgetter

import yaml
from cached_property import cached_property

from . import types
from ..const import COMPOSE_SPEC as VERSION
from ..const import COMPOSEFILE_V1 as V1
from ..utils import build_string_dict
from ..utils import json_hash
from ..utils import parse_bytes
from ..utils import parse_nanoseconds_int
from ..utils import splitdrive
from ..version import ComposeVersion
from .environment import env_vars_from_file
from .environment import Environment
from .environment import split_env
from .errors import CircularReference
from .errors import ComposeFileNotFound
from .errors import ConfigurationError
from .errors import DuplicateOverrideFileFound
from .errors import VERSION_EXPLANATION
from .interpolation import interpolate_environment_variables
from .sort_services import get_container_name_from_network_mode
from .sort_services import get_service_name_from_network_mode
from .sort_services import sort_service_dicts
from .types import MountSpec
from .types import parse_extra_hosts
from .types import parse_restart_spec
from .types import SecurityOpt
from .types import ServiceLink
from .types import ServicePort
from .types import VolumeFromSpec
from .types import VolumeSpec
from .validation import match_named_volumes
from .validation import validate_against_config_schema
from .validation import validate_config_section
from .validation import validate_cpu
from .validation import validate_credential_spec
from .validation import validate_depends_on
from .validation import validate_extends_file_path
from .validation import validate_healthcheck
from .validation import validate_ipc_mode
from .validation import validate_links
from .validation import validate_network_mode
from .validation import validate_pid_mode
from .validation import validate_service_constraints
from .validation import validate_top_level_object
from .validation import validate_ulimits


DOCKER_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cgroup_parent',
    'command',
    'cpu_count',
    'cpu_percent',
    'cpu_period',
    'cpu_quota',
    'cpu_rt_period',
    'cpu_rt_runtime',
    'cpu_shares',
    'cpus',
    'cpuset',
    'detach',
    'device_cgroup_rules',
    'devices',
    'dns',
    'dns_search',
    'dns_opt',
    'domainname',
    'entrypoint',
    'env_file',
    'environment',
    'extra_hosts',
    'group_add',
    'hostname',
    'healthcheck',
    'image',
    'ipc',
    'isolation',
    'labels',
    'links',
    'mac_address',
    'mem_limit',
    'mem_reservation',
    'memswap_limit',
    'mem_swappiness',
    'net',
    'oom_score_adj',
    'oom_kill_disable',
    'pid',
    'ports',
    'privileged',
    'read_only',
    'restart',
    'runtime',
    'secrets',
    'security_opt',
    'shm_size',
    'pids_limit',
    'stdin_open',
    'stop_signal',
    'sysctls',
    'tty',
    'user',
    'userns_mode',
    'volume_driver',
    'volumes',
    'volumes_from',
    'working_dir',
]

ALLOWED_KEYS = DOCKER_CONFIG_KEYS + [
    'blkio_config',
    'build',
    'container_name',
    'credential_spec',
    'dockerfile',
    'init',
    'log_driver',
    'log_opt',
    'logging',
    'network_mode',
    'platform',
    'profiles',
    'scale',
    'stop_grace_period',
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

DEFAULT_OVERRIDE_FILENAMES = ('docker-compose.override.yml', 'docker-compose.override.yaml')


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
        return super().__new__(
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
    def config_version(self):
        version = self.config.get('version', None)
        if isinstance(version, dict):
            return V1
        return ComposeVersion(version) if version else self.version

    @cached_property
    def version(self):
        version = self.config.get('version', None)
        if not version:
            # no version is specified in the config file
            services = self.config.get('services', None)
            networks = self.config.get('networks', None)
            volumes = self.config.get('volumes', None)
            if services or networks or volumes:
                # validate V2/V3 structure
                for section in ['services', 'networks', 'volumes']:
                    validate_config_section(
                        self.filename, self.config.get(section, {}), section)
                return VERSION

            # validate V1 structure
            validate_config_section(
                self.filename, self.config, 'services')
            return V1

        if isinstance(version, dict):
            log.warning('Unexpected type for "version" key in "{}". Assuming '
                        '"version" is the name of a service, and defaulting to '
                        'Compose file version {}.'.format(self.filename, V1))
            return V1

        if not isinstance(version, str):
            raise ConfigurationError(
                'Version in "{}" is invalid - it should be a string.'
                .format(self.filename))

        if isinstance(version, str):
            version_pattern = re.compile(r"^[1-3]+(\.\d+)?$")
            if not version_pattern.match(version):
                raise ConfigurationError(
                    'Version "{}" in "{}" is invalid.'
                    .format(version, self.filename))

        if version.startswith("1"):
            raise ConfigurationError(
                'Version in "{}" is invalid. {}'
                .format(self.filename, VERSION_EXPLANATION)
            )

        return VERSION

    def get_service(self, name):
        return self.get_service_dicts()[name]

    def get_service_dicts(self):
        if self.version == V1:
            return self.config
        return self.config.get('services', {})

    def get_volumes(self):
        return {} if self.version == V1 else self.config.get('volumes', {})

    def get_networks(self):
        return {} if self.version == V1 else self.config.get('networks', {})

    def get_secrets(self):
        return {} if self.version == V1 else self.config.get('secrets', {})

    def get_configs(self):
        return {} if self.version == V1 else self.config.get('configs', {})


class Config(namedtuple('_Config', 'config_version version services volumes networks secrets configs')):
    """
    :param config_version: configuration file version
    :type  config_version: int
    :param version: configuration version
    :type  version: int
    :param services: List of service description dictionaries
    :type  services: :class:`list`
    :param volumes: Dictionary mapping volume names to description dictionaries
    :type  volumes: :class:`dict`
    :param networks: Dictionary mapping network names to description dictionaries
    :type  networks: :class:`dict`
    :param secrets: Dictionary mapping secret names to description dictionaries
    :type secrets: :class:`dict`
    :param configs: Dictionary mapping config names to description dictionaries
    :type configs: :class:`dict`
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


def find(base_dir, filenames, environment, override_dir=None):
    if filenames == ['-']:
        return ConfigDetails(
            os.path.abspath(override_dir) if override_dir else os.getcwd(),
            [ConfigFile(None, yaml.safe_load(sys.stdin))],
            environment
        )

    if filenames:
        filenames = [os.path.join(base_dir, f) for f in filenames]
    else:
        filenames = get_default_config_files(base_dir)

    log.debug("Using configuration files: {}".format(",".join(filenames)))
    return ConfigDetails(
        override_dir if override_dir else os.path.dirname(filenames[0]),
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
                "Version mismatch: file {} specifies version {} but "
                "extension file {} uses version {}".format(
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
        log.warning("Found multiple config files with supported names: %s", ", ".join(candidates))
        log.warning("Using %s\n", winner)

    return [os.path.join(path, winner)] + get_default_override_file(path)


def get_default_override_file(path):
    override_files_in_path = [os.path.join(path, override_filename) for override_filename
                              in DEFAULT_OVERRIDE_FILENAMES
                              if os.path.exists(os.path.join(path, override_filename))]
    if len(override_files_in_path) > 1:
        raise DuplicateOverrideFileFound(override_files_in_path)
    return override_files_in_path


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


def check_swarm_only_config(service_dicts):
    warning_template = (
        "Some services ({services}) use the '{key}' key, which will be ignored. "
        "Compose does not support '{key}' configuration - use "
        "`docker stack deploy` to deploy to a swarm."
    )
    key = 'configs'
    services = [s for s in service_dicts if s.get(key)]
    if services:
        log.warning(
            warning_template.format(
                services=", ".join(sorted(s['name'] for s in services)),
                key=key
            )
        )


def load(config_details, interpolate=True):
    """Load the configuration from a working directory and a list of
    configuration files.  Files are loaded in order, and merged on top
    of each other to create the final configuration.

    Return a fully interpolated, extended and validated configuration.
    """

    # validate against latest version and if fails do it against v1 schema
    validate_config_version(config_details.config_files)

    processed_files = [
        process_config_file(config_file, config_details.environment, interpolate=interpolate)
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
    secrets = load_mapping(
        config_details.config_files, 'get_secrets', 'Secret', config_details.working_dir
    )
    configs = load_mapping(
        config_details.config_files, 'get_configs', 'Config', config_details.working_dir
    )
    service_dicts = load_services(config_details, main_file, interpolate=interpolate)

    if main_file.version != V1:
        for service_dict in service_dicts:
            match_named_volumes(service_dict, volumes)

    check_swarm_only_config(service_dicts)

    return Config(main_file.config_version, main_file.version,
                  service_dicts, volumes, networks, secrets, configs)


def load_mapping(config_files, get_func, entity_type, working_dir=None):
    mapping = {}

    for config_file in config_files:
        for name, config in getattr(config_file, get_func)().items():
            mapping[name] = config or {}
            if not config:
                continue

            external = config.get('external')
            if external:
                validate_external(entity_type, name, config, config_file.version)
                if isinstance(external, dict):
                    config['name'] = external.get('name')
                elif not config.get('name'):
                    config['name'] = name

            if 'labels' in config:
                config['labels'] = parse_labels(config['labels'])

            if 'file' in config:
                config['file'] = expand_path(working_dir, config['file'])

            if 'driver_opts' in config:
                config['driver_opts'] = build_string_dict(
                    config['driver_opts']
                )
                device = format_device_option(entity_type, config)
                if device:
                    config['driver_opts']['device'] = device
    return mapping


def format_device_option(entity_type, config):
    if entity_type != 'Volume':
        return
    # default driver is 'local'
    driver = config.get('driver', 'local')
    if driver != 'local':
        return
    o = config['driver_opts'].get('o')
    device = config['driver_opts'].get('device')
    if o and o == 'bind' and device:
        fullpath = os.path.abspath(os.path.expanduser(device))
        return fullpath


def validate_external(entity_type, name, config, version):
    for k in config.keys():
        if entity_type == 'Network' and k == 'driver':
            continue
        if k not in ['external', 'name']:
            raise ConfigurationError(
                "{} {} declared as external but specifies additional attributes "
                "({}).".format(
                    entity_type, name, ', '.join(k for k in config if k != 'external')))


def load_services(config_details, config_file, interpolate=True):
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
        validate_service(service_config, service_names, config_file)
        service_dict = finalize_service(
            service_config,
            service_names,
            config_file.version,
            config_details.environment,
            interpolate
        )
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

    service_config = functools.reduce(merge_services, service_configs)

    return build_services(service_config)


def interpolate_config_section(config_file, config, section, environment):
    return interpolate_environment_variables(
        config_file.version,
        config,
        section,
        environment
    )


def process_config_section(config_file, config, section, environment, interpolate):
    validate_config_section(config_file.filename, config, section)
    if interpolate:
        return interpolate_environment_variables(
            config_file.version,
            config,
            section,
            environment
            )
    else:
        return config


def process_config_file(config_file, environment, service_name=None, interpolate=True):
    services = process_config_section(
        config_file,
        config_file.get_service_dicts(),
        'service',
        environment,
        interpolate,
    )

    if config_file.version > V1:
        processed_config = dict(config_file.config)
        processed_config['services'] = services
        processed_config['volumes'] = process_config_section(
            config_file,
            config_file.get_volumes(),
            'volume',
            environment,
            interpolate,
        )
        processed_config['networks'] = process_config_section(
            config_file,
            config_file.get_networks(),
            'network',
            environment,
            interpolate,
        )
        processed_config['secrets'] = process_config_section(
            config_file,
            config_file.get_secrets(),
            'secret',
            environment,
            interpolate,
        )
        processed_config['configs'] = process_config_section(
            config_file,
            config_file.get_configs(),
            'config',
            environment,
            interpolate,
        )
    else:
        processed_config = services

    config_file = config_file._replace(config=processed_config)
    validate_against_config_schema(config_file, config_file.version)

    if service_name and service_name not in services:
        raise ConfigurationError(
            "Cannot extend service '{}' in {}: Service not found".format(
                service_name, config_file.filename))

    return config_file


class ServiceExtendsResolver:
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

        if config_path == os.path.abspath(self.config_file.filename):
            try:
                service_config = self.config_file.get_service(service_name)
            except KeyError:
                raise ConfigurationError(
                    "Cannot extend service '{}' in {}: Service not found".format(
                        service_name, config_path)
                )
        else:
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


def resolve_environment(service_dict, environment=None, interpolate=True):
    """Unpack any environment variables from an env_file, if set.
    Interpolate environment values if set.
    """
    env = {}
    for env_file in service_dict.get('env_file', []):
        env.update(env_vars_from_file(env_file, interpolate))

    env.update(parse_environment(service_dict.get('environment')))
    return dict(resolve_env_var(k, v, environment) for k, v in env.items())


def resolve_build_args(buildargs, environment):
    args = parse_build_arguments(buildargs)
    return dict(resolve_env_var(k, v, environment) for k, v in args.items())


def validate_extended_service_dict(service_dict, filename, service):
    error_prefix = "Cannot extend service '{}' in {}:".format(service, filename)

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


def validate_service(service_config, service_names, config_file):
    def build_image():
        args = sys.argv[1:]
        if 'pull' in args:
            return False

        if '--no-build' in args:
            return False

        return True

    service_dict, service_name = service_config.config, service_config.name
    validate_service_constraints(service_dict, service_name, config_file)

    if build_image():
        # We only care about valid paths when actually building images
        validate_paths(service_dict)

    validate_cpu(service_config)
    validate_ulimits(service_config)
    validate_ipc_mode(service_config, service_names)
    validate_network_mode(service_config, service_names)
    validate_pid_mode(service_config, service_names)
    validate_depends_on(service_config, service_names)
    validate_links(service_config, service_names)
    validate_healthcheck(service_config)
    validate_credential_spec(service_config)

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
        process_build_section(service_dict, working_dir)

    if 'volumes' in service_dict and service_dict.get('volume_driver') is None:
        service_dict['volumes'] = resolve_volume_paths(working_dir, service_dict)

    if 'sysctls' in service_dict:
        service_dict['sysctls'] = build_string_dict(parse_sysctls(service_dict['sysctls']))

    if 'labels' in service_dict:
        service_dict['labels'] = parse_labels(service_dict['labels'])

    service_dict = process_depends_on(service_dict)

    for field in ['dns', 'dns_search', 'tmpfs']:
        if field in service_dict:
            service_dict[field] = to_list(service_dict[field])

    service_dict = process_security_opt(process_blkio_config(process_ports(
        process_healthcheck(service_dict)
    )))

    return service_dict


def process_build_section(service_dict, working_dir):
    if isinstance(service_dict['build'], str):
        service_dict['build'] = resolve_build_path(working_dir, service_dict['build'])
    elif isinstance(service_dict['build'], dict):
        if 'context' in service_dict['build']:
            path = service_dict['build']['context']
            service_dict['build']['context'] = resolve_build_path(working_dir, path)
        if 'labels' in service_dict['build']:
            service_dict['build']['labels'] = parse_labels(service_dict['build']['labels'])


def process_ports(service_dict):
    if 'ports' not in service_dict:
        return service_dict

    ports = []
    for port_definition in service_dict['ports']:
        if isinstance(port_definition, ServicePort):
            ports.append(port_definition)
        else:
            ports.extend(ServicePort.parse(port_definition))
    service_dict['ports'] = ports
    return service_dict


def process_depends_on(service_dict):
    if 'depends_on' in service_dict and not isinstance(service_dict['depends_on'], dict):
        service_dict['depends_on'] = {
            svc: {'condition': 'service_started'} for svc in service_dict['depends_on']
        }
    return service_dict


def process_blkio_config(service_dict):
    if not service_dict.get('blkio_config'):
        return service_dict

    for field in ['device_read_bps', 'device_write_bps']:
        if field in service_dict['blkio_config']:
            for v in service_dict['blkio_config'].get(field, []):
                rate = v.get('rate', 0)
                v['rate'] = parse_bytes(rate)
                if v['rate'] is None:
                    raise ConfigurationError('Invalid format for bytes value: "{}"'.format(rate))

    for field in ['device_read_iops', 'device_write_iops']:
        if field in service_dict['blkio_config']:
            for v in service_dict['blkio_config'].get(field, []):
                try:
                    v['rate'] = int(v.get('rate', 0))
                except ValueError:
                    raise ConfigurationError(
                        'Invalid IOPS value: "{}". Must be a positive integer.'.format(v.get('rate'))
                    )

    return service_dict


def process_healthcheck(service_dict):
    if 'healthcheck' not in service_dict:
        return service_dict

    hc = service_dict['healthcheck']

    if 'disable' in hc:
        del hc['disable']
        hc['test'] = ['NONE']

    for field in ['interval', 'timeout', 'start_period']:
        if field not in hc or isinstance(hc[field], int):
            continue
        hc[field] = parse_nanoseconds_int(hc[field])

    return service_dict


def finalize_service_volumes(service_dict, environment):
    if 'volumes' in service_dict:
        finalized_volumes = []
        normalize = environment.get_boolean('COMPOSE_CONVERT_WINDOWS_PATHS')
        win_host = environment.get_boolean('COMPOSE_FORCE_WINDOWS_HOST')
        for v in service_dict['volumes']:
            if isinstance(v, dict):
                finalized_volumes.append(MountSpec.parse(v, normalize, win_host))
            else:
                finalized_volumes.append(VolumeSpec.parse(v, normalize, win_host))

        duplicate_mounts = []
        mounts = [v.as_volume_spec() if isinstance(v, MountSpec) else v for v in finalized_volumes]
        for mount in mounts:
            if list(map(attrgetter('internal'), mounts)).count(mount.internal) > 1:
                duplicate_mounts.append(mount.repr())

        if duplicate_mounts:
            raise ConfigurationError("Duplicate mount points: [%s]" % (
                ', '.join(duplicate_mounts)))

        service_dict['volumes'] = finalized_volumes

    return service_dict


def finalize_service(service_config, service_names, version, environment,
                     interpolate=True):
    service_dict = dict(service_config.config)

    if 'environment' in service_dict or 'env_file' in service_dict:
        service_dict['environment'] = resolve_environment(service_dict, environment, interpolate)
        service_dict.pop('env_file', None)

    if 'volumes_from' in service_dict:
        service_dict['volumes_from'] = [
            VolumeFromSpec.parse(vf, service_names, version)
            for vf in service_dict['volumes_from']
        ]

    service_dict = finalize_service_volumes(service_dict, environment)

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

    if 'secrets' in service_dict:
        service_dict['secrets'] = [
            types.ServiceSecret.parse(s) for s in service_dict['secrets']
        ]

    if 'configs' in service_dict:
        service_dict['configs'] = [
            types.ServiceConfig.parse(c) for c in service_dict['configs']
        ]

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

    def merge_mapping(self, field, parse_func=None):
        if not self.needs_merge(field):
            return

        if parse_func is None:
            def parse_func(m):
                return m or {}

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
    md.merge_mapping('ulimits', parse_flat_dict)
    md.merge_mapping('sysctls', parse_sysctls)
    md.merge_mapping('depends_on', parse_depends_on)
    md.merge_mapping('storage_opt', parse_flat_dict)
    md.merge_sequence('links', ServiceLink.parse)
    md.merge_sequence('secrets', types.ServiceSecret.parse)
    md.merge_sequence('configs', types.ServiceConfig.parse)
    md.merge_sequence('security_opt', types.SecurityOpt.parse)
    md.merge_mapping('extra_hosts', parse_extra_hosts)

    md.merge_field('networks', merge_networks, default={})
    for field in ['volumes', 'devices']:
        md.merge_field(field, merge_path_mappings)

    for field in [
        'cap_add', 'cap_drop', 'expose', 'external_links',
        'volumes_from', 'device_cgroup_rules', 'profiles',
    ]:
        md.merge_field(field, merge_unique_items_lists, default=[])

    for field in ['dns', 'dns_search', 'env_file', 'tmpfs']:
        md.merge_field(field, merge_list_or_string)

    md.merge_field('logging', merge_logging, default={})
    merge_ports(md, base, override)
    md.merge_field('blkio_config', merge_blkio_config, default={})
    md.merge_field('healthcheck', merge_healthchecks, default={})
    md.merge_field('deploy', merge_deploy, default={})

    for field in set(ALLOWED_KEYS) - set(md):
        md.merge_scalar(field)

    if version == V1:
        legacy_v1_merge_image_or_build(md, base, override)
    elif md.needs_merge('build'):
        md['build'] = merge_build(md, base, override)

    return dict(md)


def merge_unique_items_lists(base, override):
    override = (str(o) for o in override)
    base = (str(b) for b in base)
    return sorted(set(chain(base, override)))


def merge_healthchecks(base, override):
    if override.get('disabled') is True:
        return override
    result = base.copy()
    result.update(override)
    return result


def merge_ports(md, base, override):
    def parse_sequence_func(seq):
        acc = [s for item in seq for s in ServicePort.parse(item)]
        return to_mapping(acc, 'merge_field')

    field = 'ports'

    if not md.needs_merge(field):
        return

    merged = parse_sequence_func(md.base.get(field, []))
    merged.update(parse_sequence_func(md.override.get(field, [])))
    md[field] = [item for item in sorted(merged.values(), key=attrgetter("target"))]


def merge_build(output, base, override):
    def to_dict(service):
        build_config = service.get('build', {})
        if isinstance(build_config, str):
            return {'context': build_config}
        return build_config

    md = MergeDict(to_dict(base), to_dict(override))
    md.merge_scalar('context')
    md.merge_scalar('dockerfile')
    md.merge_scalar('network')
    md.merge_scalar('target')
    md.merge_scalar('shm_size')
    md.merge_scalar('isolation')
    md.merge_mapping('args', parse_build_arguments)
    md.merge_field('cache_from', merge_unique_items_lists, default=[])
    md.merge_mapping('labels', parse_labels)
    md.merge_mapping('extra_hosts', parse_extra_hosts)
    return dict(md)


def merge_deploy(base, override):
    md = MergeDict(base or {}, override or {})
    md.merge_scalar('mode')
    md.merge_scalar('endpoint_mode')
    md.merge_scalar('replicas')
    md.merge_mapping('labels', parse_labels)
    md.merge_mapping('update_config')
    md.merge_mapping('rollback_config')
    md.merge_mapping('restart_policy')
    if md.needs_merge('resources'):
        resources_md = MergeDict(md.base.get('resources') or {}, md.override.get('resources') or {})
        resources_md.merge_mapping('limits')
        resources_md.merge_field('reservations', merge_reservations, default={})
        md['resources'] = dict(resources_md)
    if md.needs_merge('placement'):
        placement_md = MergeDict(md.base.get('placement') or {}, md.override.get('placement') or {})
        placement_md.merge_scalar('max_replicas_per_node')
        placement_md.merge_field('constraints', merge_unique_items_lists, default=[])
        placement_md.merge_field('preferences', merge_unique_objects_lists, default=[])
        md['placement'] = dict(placement_md)

    return dict(md)


def merge_networks(base, override):
    merged_networks = {}
    all_network_names = set(base) | set(override)
    base = {k: {} for k in base} if isinstance(base, list) else base
    override = {k: {} for k in override} if isinstance(override, list) else override
    for network_name in all_network_names:
        md = MergeDict(base.get(network_name) or {}, override.get(network_name) or {})
        md.merge_field('aliases', merge_unique_items_lists, [])
        md.merge_field('link_local_ips', merge_unique_items_lists, [])
        md.merge_scalar('priority')
        md.merge_scalar('ipv4_address')
        md.merge_scalar('ipv6_address')
        merged_networks[network_name] = dict(md)
    return merged_networks


def merge_reservations(base, override):
    md = MergeDict(base, override)
    md.merge_scalar('cpus')
    md.merge_scalar('memory')
    md.merge_sequence('generic_resources', types.GenericResource.parse)
    md.merge_field('devices', merge_unique_objects_lists, default=[])
    return dict(md)


def merge_unique_objects_lists(base, override):
    result = {json_hash(i): i for i in base + override}
    return [i[1] for i in sorted(((k, v) for k, v in result.items()), key=itemgetter(0))]


def merge_blkio_config(base, override):
    md = MergeDict(base, override)
    md.merge_scalar('weight')

    def merge_blkio_limits(base, override):
        get_path = itemgetter('path')
        index = {get_path(b): b for b in base}
        index.update((get_path(o), o) for o in override)

        return sorted(index.values(), key=get_path)

    for field in [
            "device_read_bps", "device_read_iops", "device_write_bps",
            "device_write_iops", "weight_device",
    ]:
        md.merge_field(field, merge_blkio_limits, default=[])

    return dict(md)


def merge_logging(base, override):
    md = MergeDict(base, override)
    md.merge_scalar('driver')
    if md.get('driver') == base.get('driver') or base.get('driver') is None:
        md.merge_mapping('options', lambda m: m or {})
    elif override.get('options'):
        md['options'] = override.get('options', {})
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


def merge_labels(base, override):
    labels = parse_labels(base)
    labels.update(parse_labels(override))
    return labels


def split_kv(kvpair):
    if '=' in kvpair:
        return kvpair.split('=', 1)
    else:
        return kvpair, ''


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
parse_labels = functools.partial(parse_dict_or_list, split_kv, 'labels')
parse_networks = functools.partial(parse_dict_or_list, lambda k: (k, None), 'networks')
parse_sysctls = functools.partial(parse_dict_or_list, split_kv, 'sysctls')
parse_depends_on = functools.partial(
    parse_dict_or_list, lambda k: (k, {'condition': 'service_started'}), 'depends_on'
)


def parse_flat_dict(d):
    if not d:
        return {}

    if isinstance(d, dict):
        return dict(d)

    raise ConfigurationError("Invalid type: expected mapping")


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
    if isinstance(volume, dict):
        if volume.get('source', '').startswith(('.', '~')) and volume['type'] == 'bind':
            volume['source'] = expand_path(working_dir, volume['source'])
        return volume

    mount_params = None
    container_path, mount_params = split_path_mapping(volume)

    if mount_params is not None:
        host_path, mode = mount_params
        if host_path is None:
            return container_path
        if host_path.startswith('.'):
            host_path = expand_path(working_dir, host_path)
        host_path = os.path.expanduser(host_path)
        return "{}:{}{}".format(host_path, container_path, (':' + mode if mode else ''))

    return container_path


def normalize_build(service_dict, working_dir, environment):

    if 'build' in service_dict:
        build = {}
        # Shortcut where specifying a string is treated as the build context
        if isinstance(service_dict['build'], str):
            build['context'] = service_dict.pop('build')
        else:
            build.update(service_dict['build'])
            if 'args' in build:
                build['args'] = build_string_dict(
                    resolve_build_args(build.get('args'), environment)
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

        if isinstance(build, str):
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
    if isinstance(volume_path, dict):
        return (volume_path.get('target'), volume_path)
    drive, volume_config = splitdrive(volume_path)

    if ':' in volume_config:
        (host, container) = volume_config.split(':', 1)
        container_drive, container_path = splitdrive(container)
        mode = None
        if ':' in container_path:
            container_path, mode = container_path.rsplit(':', 1)

        return (container_drive + container_path, (drive + host, mode))
    else:
        return (volume_path, None)


def process_security_opt(service_dict):
    security_opts = service_dict.get('security_opt', [])
    result = []
    for value in security_opts:
        result.append(SecurityOpt.parse(value))
    if result:
        service_dict['security_opt'] = result
    return service_dict


def join_path_mapping(pair):
    (container, host) = pair
    if isinstance(host, dict):
        return host
    elif host is None:
        return container
    else:
        host, mode = host
        result = ":".join((host, container))
        if mode:
            result += ":" + mode
        return result


def expand_path(working_dir, path):
    return os.path.abspath(os.path.join(working_dir, os.path.expanduser(path)))


def merge_list_or_string(base, override):
    return to_list(base) + to_list(override)


def to_list(value):
    if value is None:
        return []
    elif isinstance(value, str):
        return [value]
    else:
        return value


def to_mapping(sequence, key_field):
    return {getattr(item, key_field): item for item in sequence}


def has_uppercase(name):
    return any(char in string.ascii_uppercase for char in name)


def load_yaml(filename, encoding=None, binary=True):
    try:
        with open(filename, 'rb' if binary else 'r', encoding=encoding) as fh:
            return yaml.safe_load(fh)
    except (OSError, yaml.YAMLError, UnicodeDecodeError) as e:
        if encoding is None:
            # Sometimes the user's locale sets an encoding that doesn't match
            # the YAML files. Im such cases, retry once with the "default"
            # UTF-8 encoding
            return load_yaml(filename, encoding='utf-8-sig', binary=False)
        error_name = getattr(e, '__module__', '') + '.' + e.__class__.__name__
        raise ConfigurationError("{}: {}".format(error_name, e))
