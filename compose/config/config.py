import logging
import os
import sys
from collections import namedtuple

import six
import yaml

from .errors import CircularReference
from .errors import ComposeFileNotFound
from .errors import ConfigurationError
from .interpolation import interpolate_environment_variables
from .validation import validate_against_fields_schema
from .validation import validate_against_service_schema
from .validation import validate_extended_service_exists
from .validation import validate_extends_file_path
from .validation import validate_service_names
from .validation import validate_top_level_object
from compose.cli.utils import find_candidates_in_parent_dirs


DOCKER_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cpu_shares',
    'cpuset',
    'command',
    'detach',
    'devices',
    'dns',
    'dns_search',
    'domainname',
    'entrypoint',
    'env_file',
    'environment',
    'extra_hosts',
    'hostname',
    'image',
    'labels',
    'links',
    'mac_address',
    'mem_limit',
    'memswap_limit',
    'net',
    'log_driver',
    'log_opt',
    'pid',
    'ports',
    'privileged',
    'read_only',
    'restart',
    'security_opt',
    'stdin_open',
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
    'expose',
    'external_links',
    'name',
]


SUPPORTED_FILENAMES = [
    'docker-compose.yml',
    'docker-compose.yaml',
    'fig.yml',
    'fig.yaml',
]


PATH_START_CHARS = [
    '/',
    '.',
    '~',
]


log = logging.getLogger(__name__)


ConfigDetails = namedtuple('ConfigDetails', 'config working_dir filename')


def find(base_dir, filename):
    if filename == '-':
        return ConfigDetails(yaml.safe_load(sys.stdin), os.getcwd(), None)

    if filename:
        filename = os.path.join(base_dir, filename)
    else:
        filename = get_config_path(base_dir)
    return ConfigDetails(load_yaml(filename), os.path.dirname(filename), filename)


def get_config_path(base_dir):
    (candidates, path) = find_candidates_in_parent_dirs(SUPPORTED_FILENAMES, base_dir)

    if len(candidates) == 0:
        raise ComposeFileNotFound(SUPPORTED_FILENAMES)

    winner = candidates[0]

    if len(candidates) > 1:
        log.warn("Found multiple config files with supported names: %s", ", ".join(candidates))
        log.warn("Using %s\n", winner)

    if winner == 'docker-compose.yaml':
        log.warn("Please be aware that .yml is the expected extension "
                 "in most cases, and using .yaml can cause compatibility "
                 "issues in future.\n")

    if winner.startswith("fig."):
        log.warn("%s is deprecated and will not be supported in future. "
                 "Please rename your config file to docker-compose.yml\n" % winner)

    return os.path.join(path, winner)


@validate_top_level_object
@validate_service_names
def pre_process_config(config):
    """
    Pre validation checks and processing of the config file to interpolate env
    vars returning a config dict ready to be tested against the schema.
    """
    config = interpolate_environment_variables(config)
    return config


def load(config_details):
    config, working_dir, filename = config_details

    processed_config = pre_process_config(config)
    validate_against_fields_schema(processed_config)

    service_dicts = []

    for service_name, service_dict in list(processed_config.items()):
        loader = ServiceLoader(
            working_dir=working_dir,
            filename=filename,
            service_name=service_name,
            service_dict=service_dict)
        service_dict = loader.make_service_dict()
        validate_paths(service_dict)
        service_dicts.append(service_dict)

    return service_dicts


class ServiceLoader(object):
    def __init__(self, working_dir, filename, service_name, service_dict, already_seen=None):
        if working_dir is None:
            raise Exception("No working_dir passed to ServiceLoader()")

        self.working_dir = os.path.abspath(working_dir)

        if filename:
            self.filename = os.path.abspath(filename)
        else:
            self.filename = filename
        self.already_seen = already_seen or []
        self.service_dict = service_dict.copy()
        self.service_name = service_name
        self.service_dict['name'] = service_name

    def detect_cycle(self, name):
        if self.signature(name) in self.already_seen:
            raise CircularReference(self.already_seen + [self.signature(name)])

    def make_service_dict(self):
        self.resolve_environment()
        if 'extends' in self.service_dict:
            self.validate_and_construct_extends()
            self.service_dict = self.resolve_extends()

        if not self.already_seen:
            validate_against_service_schema(self.service_dict)

        return process_container_options(self.service_dict, working_dir=self.working_dir)

    def resolve_environment(self):
        """
        Unpack any environment variables from an env_file, if set.
        Interpolate environment values if set.
        """
        if 'environment' not in self.service_dict and 'env_file' not in self.service_dict:
            return

        env = {}

        if 'env_file' in self.service_dict:
            for f in get_env_files(self.service_dict, working_dir=self.working_dir):
                env.update(env_vars_from_file(f))
            del self.service_dict['env_file']

        env.update(parse_environment(self.service_dict.get('environment')))
        env = dict(resolve_env_var(k, v) for k, v in six.iteritems(env))

        self.service_dict['environment'] = env

    def validate_and_construct_extends(self):
        validate_extends_file_path(
            self.service_name,
            self.service_dict['extends'],
            self.filename
        )
        self.extended_config_path = self.get_extended_config_path(
            self.service_dict['extends']
        )
        self.extended_service_name = self.service_dict['extends']['service']

        full_extended_config = pre_process_config(
            load_yaml(self.extended_config_path)
        )

        validate_extended_service_exists(
            self.extended_service_name,
            full_extended_config,
            self.extended_config_path
        )
        validate_against_fields_schema(full_extended_config)

        self.extended_config = full_extended_config[self.extended_service_name]

    def resolve_extends(self):
        other_working_dir = os.path.dirname(self.extended_config_path)
        other_already_seen = self.already_seen + [self.signature(self.service_name)]

        other_loader = ServiceLoader(
            working_dir=other_working_dir,
            filename=self.extended_config_path,
            service_name=self.service_name,
            service_dict=self.extended_config,
            already_seen=other_already_seen,
        )

        other_loader.detect_cycle(self.extended_service_name)
        other_service_dict = other_loader.make_service_dict()
        validate_extended_service_dict(
            other_service_dict,
            filename=self.extended_config_path,
            service=self.extended_service_name,
        )

        return merge_service_dicts(other_service_dict, self.service_dict)

    def get_extended_config_path(self, extends_options):
        """
        Service we are extending either has a value for 'file' set, which we
        need to obtain a full path too or we are extending from a service
        defined in our own file.
        """
        if 'file' in extends_options:
            extends_from_filename = extends_options['file']
            return expand_path(self.working_dir, extends_from_filename)

        return self.filename

    def signature(self, name):
        return (self.filename, name)


def validate_extended_service_dict(service_dict, filename, service):
    error_prefix = "Cannot extend service '%s' in %s:" % (service, filename)

    if 'links' in service_dict:
        raise ConfigurationError("%s services with 'links' cannot be extended" % error_prefix)

    if 'volumes_from' in service_dict:
        raise ConfigurationError("%s services with 'volumes_from' cannot be extended" % error_prefix)

    if 'net' in service_dict:
        if get_service_name_from_net(service_dict['net']) is not None:
            raise ConfigurationError("%s services with 'net: container' cannot be extended" % error_prefix)


def process_container_options(service_dict, working_dir=None):
    service_dict = service_dict.copy()

    if 'volumes' in service_dict and service_dict.get('volume_driver') is None:
        service_dict['volumes'] = resolve_volume_paths(service_dict, working_dir=working_dir)

    if 'build' in service_dict:
        service_dict['build'] = resolve_build_path(service_dict['build'], working_dir=working_dir)

    if 'labels' in service_dict:
        service_dict['labels'] = parse_labels(service_dict['labels'])

    return service_dict


def merge_service_dicts(base, override):
    d = base.copy()

    if 'environment' in base or 'environment' in override:
        d['environment'] = merge_environment(
            base.get('environment'),
            override.get('environment'),
        )

    path_mapping_keys = ['volumes', 'devices']

    for key in path_mapping_keys:
        if key in base or key in override:
            d[key] = merge_path_mappings(
                base.get(key),
                override.get(key),
            )

    if 'labels' in base or 'labels' in override:
        d['labels'] = merge_labels(
            base.get('labels'),
            override.get('labels'),
        )

    if 'image' in override and 'build' in d:
        del d['build']

    if 'build' in override and 'image' in d:
        del d['image']

    list_keys = ['ports', 'expose', 'external_links']

    for key in list_keys:
        if key in base or key in override:
            d[key] = base.get(key, []) + override.get(key, [])

    list_or_string_keys = ['dns', 'dns_search']

    for key in list_or_string_keys:
        if key in base or key in override:
            d[key] = to_list(base.get(key)) + to_list(override.get(key))

    already_merged_keys = ['environment', 'labels'] + path_mapping_keys + list_keys + list_or_string_keys

    for k in set(ALLOWED_KEYS) - set(already_merged_keys):
        if k in override:
            d[k] = override[k]

    return d


def merge_environment(base, override):
    env = parse_environment(base)
    env.update(parse_environment(override))
    return env


def get_env_files(options, working_dir=None):
    if 'env_file' not in options:
        return {}

    env_files = options.get('env_file', [])
    if not isinstance(env_files, list):
        env_files = [env_files]

    return [expand_path(working_dir, path) for path in env_files]


def parse_environment(environment):
    if not environment:
        return {}

    if isinstance(environment, list):
        return dict(split_env(e) for e in environment)

    if isinstance(environment, dict):
        return dict(environment)

    raise ConfigurationError(
        "environment \"%s\" must be a list or mapping," %
        environment
    )


def split_env(env):
    if '=' in env:
        return env.split('=', 1)
    else:
        return env, None


def resolve_env_var(key, val):
    if val is not None:
        return key, val
    elif key in os.environ:
        return key, os.environ[key]
    else:
        return key, ''


def env_vars_from_file(filename):
    """
    Read in a line delimited file of environment variables.
    """
    if not os.path.exists(filename):
        raise ConfigurationError("Couldn't find env file: %s" % filename)
    env = {}
    for line in open(filename, 'r'):
        line = line.strip()
        if line and not line.startswith('#'):
            k, v = split_env(line)
            env[k] = v
    return env


def resolve_volume_paths(service_dict, working_dir=None):
    if working_dir is None:
        raise Exception("No working_dir passed to resolve_volume_paths()")

    return [
        resolve_volume_path(v, working_dir, service_dict['name'])
        for v in service_dict['volumes']
    ]


def resolve_volume_path(volume, working_dir, service_name):
    container_path, host_path = split_path_mapping(volume)
    container_path = os.path.expanduser(container_path)

    if host_path is not None:
        if not any(host_path.startswith(c) for c in PATH_START_CHARS):
            log.warn(
                'Warning: the mapping "{0}:{1}" in the volumes config for '
                'service "{2}" is ambiguous. In a future version of Docker, '
                'it will designate a "named" volume '
                '(see https://github.com/docker/docker/pull/14242). '
                'To prevent unexpected behaviour, change it to "./{0}:{1}"'
                .format(host_path, container_path, service_name)
            )

        host_path = os.path.expanduser(host_path)
        return "%s:%s" % (expand_path(working_dir, host_path), container_path)
    else:
        return container_path


def resolve_build_path(build_path, working_dir=None):
    if working_dir is None:
        raise Exception("No working_dir passed to resolve_build_path")
    return expand_path(working_dir, build_path)


def validate_paths(service_dict):
    if 'build' in service_dict:
        build_path = service_dict['build']
        if not os.path.exists(build_path) or not os.access(build_path, os.R_OK):
            raise ConfigurationError("build path %s either does not exist or is not accessible." % build_path)


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
    return [join_path_mapping(v) for v in d.items()]


def split_path_mapping(string):
    if ':' in string:
        (host, container) = string.split(':', 1)
        return (container, host)
    else:
        return (string, None)


def join_path_mapping(pair):
    (container, host) = pair
    if host is None:
        return container
    else:
        return ":".join((host, container))


def merge_labels(base, override):
    labels = parse_labels(base)
    labels.update(parse_labels(override))
    return labels


def parse_labels(labels):
    if not labels:
        return {}

    if isinstance(labels, list):
        return dict(split_label(e) for e in labels)

    if isinstance(labels, dict):
        return labels


def split_label(label):
    if '=' in label:
        return label.split('=', 1)
    else:
        return label, ''


def expand_path(working_dir, path):
    return os.path.abspath(os.path.join(working_dir, path))


def to_list(value):
    if value is None:
        return []
    elif isinstance(value, six.string_types):
        return [value]
    else:
        return value


def get_service_name_from_net(net_config):
    if not net_config:
        return

    if not net_config.startswith('container:'):
        return

    _, net_name = net_config.split(':', 1)
    return net_name


def load_yaml(filename):
    try:
        with open(filename, 'r') as fh:
            return yaml.safe_load(fh)
    except IOError as e:
        raise ConfigurationError(six.text_type(e))
