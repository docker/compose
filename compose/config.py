import os
import yaml
import six


DOCKER_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cpu_shares',
    'command',
    'detach',
    'dns',
    'dns_search',
    'domainname',
    'entrypoint',
    'env_file',
    'environment',
    'extra_hosts',
    'hostname',
    'image',
    'links',
    'mem_limit',
    'net',
    'pid',
    'ports',
    'privileged',
    'restart',
    'stdin_open',
    'tty',
    'user',
    'volumes',
    'volumes_from',
    'working_dir',
]

ALLOWED_KEYS = DOCKER_CONFIG_KEYS + [
    'build',
    'dockerfile',
    'expose',
    'external_links',
    'name',
]

DOCKER_CONFIG_HINTS = {
    'cpu_share': 'cpu_shares',
    'add_host': 'extra_hosts',
    'hosts': 'extra_hosts',
    'extra_host': 'extra_hosts',
    'link': 'links',
    'port': 'ports',
    'privilege': 'privileged',
    'priviliged': 'privileged',
    'privilige': 'privileged',
    'volume': 'volumes',
    'workdir': 'working_dir',
}


def load(filename):
    working_dir = os.path.dirname(filename)
    return from_dictionary(load_yaml(filename), working_dir=working_dir, filename=filename)


def from_dictionary(dictionary, working_dir=None, filename=None):
    service_dicts = []

    for service_name, service_dict in list(dictionary.items()):
        if not isinstance(service_dict, dict):
            raise ConfigurationError('Service "%s" doesn\'t have any configuration options. All top level keys in your docker-compose.yml must map to a dictionary of configuration options.' % service_name)
        loader = ServiceLoader(working_dir=working_dir, filename=filename)
        service_dict = loader.make_service_dict(service_name, service_dict)
        validate_paths(service_dict)
        service_dicts.append(service_dict)

    return service_dicts


def make_service_dict(name, service_dict, working_dir=None):
    return ServiceLoader(working_dir=working_dir).make_service_dict(name, service_dict)


class ServiceLoader(object):
    def __init__(self, working_dir, filename=None, already_seen=None):
        self.working_dir = working_dir
        self.filename = filename
        self.already_seen = already_seen or []

    def make_service_dict(self, name, service_dict):
        if self.signature(name) in self.already_seen:
            raise CircularReference(self.already_seen)

        service_dict = service_dict.copy()
        service_dict['name'] = name
        service_dict = resolve_environment(service_dict, working_dir=self.working_dir)
        service_dict = self.resolve_extends(service_dict)
        return process_container_options(service_dict, working_dir=self.working_dir)

    def resolve_extends(self, service_dict):
        if 'extends' not in service_dict:
            return service_dict

        extends_options = process_extends_options(service_dict['name'], service_dict['extends'])

        if self.working_dir is None:
            raise Exception("No working_dir passed to ServiceLoader()")

        other_config_path = expand_path(self.working_dir, extends_options['file'])
        other_working_dir = os.path.dirname(other_config_path)
        other_already_seen = self.already_seen + [self.signature(service_dict['name'])]
        other_loader = ServiceLoader(
            working_dir=other_working_dir,
            filename=other_config_path,
            already_seen=other_already_seen,
        )

        other_config = load_yaml(other_config_path)
        other_service_dict = other_config[extends_options['service']]
        other_service_dict = other_loader.make_service_dict(
            service_dict['name'],
            other_service_dict,
        )
        validate_extended_service_dict(
            other_service_dict,
            filename=other_config_path,
            service=extends_options['service'],
        )

        return merge_service_dicts(other_service_dict, service_dict)

    def signature(self, name):
        return (self.filename, name)


def process_extends_options(service_name, extends_options):
    error_prefix = "Invalid 'extends' configuration for %s:" % service_name

    if not isinstance(extends_options, dict):
        raise ConfigurationError("%s must be a dictionary" % error_prefix)

    if 'service' not in extends_options:
        raise ConfigurationError(
            "%s you need to specify a service, e.g. 'service: web'" % error_prefix
        )

    for k, _ in extends_options.items():
        if k not in ['file', 'service']:
            raise ConfigurationError(
                "%s unsupported configuration option '%s'" % (error_prefix, k)
            )

    return extends_options


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
    for k in service_dict:
        if k not in ALLOWED_KEYS:
            msg = "Unsupported config option for %s service: '%s'" % (service_dict['name'], k)
            if k in DOCKER_CONFIG_HINTS:
                msg += " (did you mean '%s'?)" % DOCKER_CONFIG_HINTS[k]
            raise ConfigurationError(msg)

    service_dict = service_dict.copy()

    if 'volumes' in service_dict:
        service_dict['volumes'] = resolve_host_paths(service_dict['volumes'], working_dir=working_dir)

    if 'build' in service_dict:
        service_dict['build'] = resolve_build_path(service_dict['build'], working_dir=working_dir)

    return service_dict


def merge_service_dicts(base, override):
    d = base.copy()

    if 'environment' in base or 'environment' in override:
        d['environment'] = merge_environment(
            base.get('environment'),
            override.get('environment'),
        )

    if 'volumes' in base or 'volumes' in override:
        d['volumes'] = merge_volumes(
            base.get('volumes'),
            override.get('volumes'),
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

    already_merged_keys = ['environment', 'volumes'] + list_keys + list_or_string_keys

    for k in set(ALLOWED_KEYS) - set(already_merged_keys):
        if k in override:
            d[k] = override[k]

    return d


def merge_environment(base, override):
    env = parse_environment(base)
    env.update(parse_environment(override))
    return env


def parse_links(links):
    return dict(parse_link(l) for l in links)


def parse_link(link):
    if ':' in link:
        source, alias = link.split(':', 1)
        return (alias, source)
    else:
        return (link, link)


def get_env_files(options, working_dir=None):
    if 'env_file' not in options:
        return {}

    if working_dir is None:
        raise Exception("No working_dir passed to get_env_files()")

    env_files = options.get('env_file', [])
    if not isinstance(env_files, list):
        env_files = [env_files]

    return [expand_path(working_dir, path) for path in env_files]


def resolve_environment(service_dict, working_dir=None):
    service_dict = service_dict.copy()

    if 'environment' not in service_dict and 'env_file' not in service_dict:
        return service_dict

    env = {}

    if 'env_file' in service_dict:
        for f in get_env_files(service_dict, working_dir=working_dir):
            env.update(env_vars_from_file(f))
        del service_dict['env_file']

    env.update(parse_environment(service_dict.get('environment')))
    env = dict(resolve_env_var(k, v) for k, v in six.iteritems(env))

    service_dict['environment'] = env
    return service_dict


def parse_environment(environment):
    if not environment:
        return {}

    if isinstance(environment, list):
        return dict(split_env(e) for e in environment)

    if isinstance(environment, dict):
        return environment

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


def resolve_host_paths(volumes, working_dir=None):
    if working_dir is None:
        raise Exception("No working_dir passed to resolve_host_paths()")

    return [resolve_host_path(v, working_dir) for v in volumes]


def resolve_host_path(volume, working_dir):
    container_path, host_path = split_volume(volume)
    if host_path is not None:
        host_path = os.path.expanduser(host_path)
        host_path = os.path.expandvars(host_path)
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


def merge_volumes(base, override):
    d = dict_from_volumes(base)
    d.update(dict_from_volumes(override))
    return volumes_from_dict(d)


def dict_from_volumes(volumes):
    if volumes:
        return dict(split_volume(v) for v in volumes)
    else:
        return {}


def volumes_from_dict(d):
    return [join_volume(v) for v in d.items()]


def split_volume(string):
    if ':' in string:
        (host, container) = string.split(':', 1)
        return (container, host)
    else:
        return (string, None)


def join_volume(pair):
    (container, host) = pair
    if host is None:
        return container
    else:
        return ":".join((host, container))


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


class ConfigurationError(Exception):
    def __init__(self, msg):
        self.msg = msg

    def __str__(self):
        return self.msg


class CircularReference(ConfigurationError):
    def __init__(self, trail):
        self.trail = trail

    @property
    def msg(self):
        lines = [
            "{} in {}".format(service_name, filename)
            for (filename, service_name) in self.trail
        ]
        return "Circular reference:\n  {}".format("\n  extends ".join(lines))
