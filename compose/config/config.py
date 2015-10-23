import codecs
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
from .validation import validate_top_level_object


DOCKER_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cgroup_parent',
    'command',
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
    'hostname',
    'image',
    'ipc',
    'labels',
    'links',
    'log_driver',
    'log_opt',
    'mac_address',
    'mem_limit',
    'memswap_limit',
    'net',
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

DEFAULT_OVERRIDE_FILENAME = 'docker-compose.override.yml'

log = logging.getLogger(__name__)


class ConfigDetails(namedtuple('_ConfigDetails', 'working_dir config_files')):
    """
    :param working_dir: the directory to use for relative paths in the config
    :type  working_dir: string
    :param config_files: list of configuration files to load
    :type  config_files: list of :class:`ConfigFile`
     """


class ConfigFile(namedtuple('_ConfigFile', 'filename config')):
    """
    :param filename: filename of the config file
    :type  filename: string
    :param config: contents of the config file
    :type  config: :class:`dict`
    """


def find(base_dir, filenames):
    if filenames == ['-']:
        return ConfigDetails(
            os.getcwd(),
            [ConfigFile(None, yaml.safe_load(sys.stdin))])

    if filenames:
        filenames = [os.path.join(base_dir, f) for f in filenames]
    else:
        filenames = get_default_config_files(base_dir)

    log.debug("Using configuration files: {}".format(",".join(filenames)))
    return ConfigDetails(
        os.path.dirname(filenames[0]),
        [ConfigFile(f, load_yaml(f)) for f in filenames])


def get_default_config_files(base_dir):
    (candidates, path) = find_candidates_in_parent_dirs(SUPPORTED_FILENAMES, base_dir)

    if not candidates:
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

    def build_service(filename, service_name, service_dict):
        loader = ServiceLoader(
            config_details.working_dir,
            filename,
            service_name,
            service_dict)
        service_dict = loader.make_service_dict()
        validate_paths(service_dict)
        return service_dict

    def load_file(filename, config):
        processed_config = interpolate_environment_variables(config)
        validate_against_fields_schema(processed_config)
        return [
            build_service(filename, name, service_config)
            for name, service_config in processed_config.items()
        ]

    def merge_services(base, override):
        all_service_names = set(base) | set(override)
        return {
            name: merge_service_dicts_from_files(
                base.get(name, {}),
                override.get(name, {}))
            for name in all_service_names
        }

    config_file = config_details.config_files[0]
    validate_top_level_object(config_file.config)
    for next_file in config_details.config_files[1:]:
        validate_top_level_object(next_file.config)

        config_file = ConfigFile(
            config_file.filename,
            merge_services(config_file.config, next_file.config))

    return load_file(config_file.filename, config_file.config)


class ServiceLoader(object):
    def __init__(
        self,
        working_dir,
        filename,
        service_name,
        service_dict,
        already_seen=None
    ):
        if working_dir is None:
            raise ValueError("No working_dir passed to ServiceLoader()")

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
        service_dict = dict(self.service_dict)
        env = resolve_environment(self.working_dir, self.service_dict)
        if env:
            service_dict['environment'] = env
            service_dict.pop('env_file', None)

        if 'extends' in service_dict:
            service_dict = self.resolve_extends(*self.validate_and_construct_extends())

        if not self.already_seen:
            validate_against_service_schema(service_dict, self.service_name)

        return process_container_options(service_dict, working_dir=self.working_dir)

    def validate_and_construct_extends(self):
        extends = self.service_dict['extends']
        if not isinstance(extends, dict):
            extends = {'service': extends}

        validate_extends_file_path(self.service_name, extends, self.filename)
        config_path = self.get_extended_config_path(extends)
        service_name = extends['service']

        config = load_yaml(config_path)
        validate_top_level_object(config)
        full_extended_config = interpolate_environment_variables(config)

        validate_extended_service_exists(
            service_name,
            full_extended_config,
            config_path
        )
        validate_against_fields_schema(full_extended_config)

        service_config = full_extended_config[service_name]
        return config_path, service_config, service_name

    def resolve_extends(self, extended_config_path, service_config, service_name):
        other_working_dir = os.path.dirname(extended_config_path)
        other_already_seen = self.already_seen + [self.signature(self.service_name)]

        other_loader = ServiceLoader(
            other_working_dir,
            extended_config_path,
            self.service_name,
            service_config,
            already_seen=other_already_seen,
        )

        other_loader.detect_cycle(service_name)
        other_service_dict = other_loader.make_service_dict()
        validate_extended_service_dict(
            other_service_dict,
            extended_config_path,
            service_name,
        )

        return merge_service_dicts(other_service_dict, self.service_dict)

    def get_extended_config_path(self, extends_options):
        """Service we are extending either has a value for 'file' set, which we
        need to obtain a full path too or we are extending from a service
        defined in our own file.
        """
        if 'file' in extends_options:
            return expand_path(self.working_dir, extends_options['file'])
        return self.filename

    def signature(self, name):
        return self.filename, name


def resolve_environment(working_dir, service_dict):
    """Unpack any environment variables from an env_file, if set.
    Interpolate environment values if set.
    """
    if 'environment' not in service_dict and 'env_file' not in service_dict:
        return {}

    env = {}
    if 'env_file' in service_dict:
        for env_file in get_env_files(service_dict, working_dir=working_dir):
            env.update(env_vars_from_file(env_file))

    env.update(parse_environment(service_dict.get('environment')))
    return dict(resolve_env_var(k, v) for k, v in six.iteritems(env))


def validate_extended_service_dict(service_dict, filename, service):
    error_prefix = "Cannot extend service '%s' in %s:" % (service, filename)

    if 'links' in service_dict:
        raise ConfigurationError(
            "%s services with 'links' cannot be extended" % error_prefix)

    if 'volumes_from' in service_dict:
        raise ConfigurationError(
            "%s services with 'volumes_from' cannot be extended" % error_prefix)

    if 'net' in service_dict:
        if get_service_name_from_net(service_dict['net']) is not None:
            raise ConfigurationError(
                "%s services with 'net: container' cannot be extended" % error_prefix)


def process_container_options(service_dict, working_dir=None):
    service_dict = service_dict.copy()

    if 'volumes' in service_dict and service_dict.get('volume_driver') is None:
        service_dict['volumes'] = resolve_volume_paths(service_dict, working_dir=working_dir)

    if 'build' in service_dict:
        service_dict['build'] = resolve_build_path(service_dict['build'], working_dir=working_dir)

    if 'labels' in service_dict:
        service_dict['labels'] = parse_labels(service_dict['labels'])

    return service_dict


def merge_service_dicts_from_files(base, override):
    """When merging services from multiple files we need to merge the `extends`
    field. This is not handled by `merge_service_dicts()` which is used to
    perform the `extends`.
    """
    new_service = merge_service_dicts(base, override)
    if 'extends' in override:
        new_service['extends'] = override['extends']
    return new_service


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
    if isinstance(env, six.binary_type):
        env = env.decode('utf-8')
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
    for line in codecs.open(filename, 'r', 'utf-8'):
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

    if host_path is not None:
        if host_path.startswith('.'):
            host_path = expand_path(working_dir, host_path)
        host_path = os.path.expanduser(host_path)
        return "{}:{}".format(host_path, container_path)
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


def split_path_mapping(volume_path):
    """
    Ascertain if the volume_path contains a host path as well as a container
    path. Using splitdrive so windows absolute paths won't cause issues with
    splitting on ':'.
    """
    # splitdrive has limitations when it comes to relative paths, so when it's
    # relative, handle special case to set the drive to ''
    if volume_path.startswith('.') or volume_path.startswith('~'):
        drive, volume_config = '', volume_path
    else:
        drive, volume_config = os.path.splitdrive(volume_path)

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
    return os.path.abspath(os.path.join(working_dir, os.path.expanduser(path)))


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
    except (IOError, yaml.YAMLError) as e:
        error_name = getattr(e, '__module__', '') + '.' + e.__class__.__name__
        raise ConfigurationError(u"{}: {}".format(error_name, e))
